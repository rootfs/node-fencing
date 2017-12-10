package controller

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/golang/glog"

	crdv1 "github.com/rootfs/node-fencing/pkg/apis/crd/v1"

	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	crdPostInitialDelay = 2 * time.Second
	crdPostFactor       = 1.2
	crdPostSteps        = 5
)

type Controller struct {
	crdClient      *rest.RESTClient
	crdScheme      *runtime.Scheme
	client         kubernetes.Interface
	nodePVMap      map[string][]*v1.PersistentVolume
	nodePVLock     *sync.Mutex
	nodeController cache.Controller
	podController  cache.Controller
}

func NewNodeFencingController(client kubernetes.Interface, crdClient *rest.RESTClient, crdScheme *runtime.Scheme) *Controller {
	c := &Controller{
		client:     client,
		nodePVMap:  make(map[string][]*v1.PersistentVolume),
		nodePVLock: &sync.Mutex{},
		crdClient:  crdClient,
		crdScheme:  crdScheme,
	}

	nodeListWatcher := cache.NewListWatchFromClient(
		client.Core().RESTClient(),
		"nodes",
		v1.NamespaceAll,
		fields.Everything())

	_, nodeController := cache.NewInformer(
		nodeListWatcher,
		&v1.Node{},
		time.Minute*60,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.onNodeAdd,
			UpdateFunc: c.onNodeUpdate,
			DeleteFunc: c.onNodeDelete,
		},
	)

	c.nodeController = nodeController

	podListWatcher := cache.NewListWatchFromClient(
		client.Core().RESTClient(),
		"pods",
		v1.NamespaceAll,
		fields.Everything())

	_, podController := cache.NewInformer(
		podListWatcher,
		&v1.Pod{},
		time.Minute*60,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.onPodAdd,
			UpdateFunc: c.onPodUpdate,
			DeleteFunc: c.onPodDelete,
		},
	)

	c.podController = podController

	return c
}

func (c *Controller) Run(ctx <-chan struct{}) {
	glog.Infof("Fence controller starting")

	go c.podController.Run(ctx)
	glog.Infof("Waiting for informer initial sync")
	wait.Poll(time.Second, 5*time.Minute, func() (bool, error) {
		return c.podController.HasSynced(), nil
	})
	if !c.podController.HasSynced() {
		glog.Errorf("pod informer initial sync timeout")
		os.Exit(1)
	}

	go c.nodeController.Run(ctx)
	glog.Infof("Waiting for informer initial sync")
	wait.Poll(time.Second, 5*time.Minute, func() (bool, error) {
		return c.nodeController.HasSynced(), nil
	})
	if !c.nodeController.HasSynced() {
		glog.Errorf("node informer initial sync timeout")
		os.Exit(1)
	}
	go c.handleExistingNodeFences(2)
}

func (c *Controller) handleExistingNodeFences(when time.Duration) {
	glog.Infof("Controller monitor is running every %d minutes", when)
	for x := range time.Tick(time.Duration(when * time.Minute)) {
		glog.Infof("Checking status of existing node fence objects: %s", x)
		var nodeFences crdv1.NodeFenceList
		err := c.crdClient.Get().Resource(crdv1.NodeFenceResourcePlural).Do().Into(&nodeFences)
		if err != nil {
			glog.Errorf("something went wrong - could not fetch nodefences - %s", err)
		} else {
			for _, nf := range nodeFences.Items {
				glog.Infof("Read %s ..", nf.Metadata.Name)
				switch nf.Status {
				case crdv1.NodeFenceConditionNew:
					// Check if one executor at least is running already
				case crdv1.NodeFenceConditionError:
					// Check if current executor failed to run - if so, initiates new executor
				case crdv1.NodeFenceConditionRunning:
					glog.Infof("Node fence is already running on: %s", nf.Hostname)
				case crdv1.NodeFenceConditionDone:
					// Check if host is up, if not - move to power-management step
					nf.Status = crdv1.NodeFenceConditionNew
					nf.Step = crdv1.NodeFenceStepPowerManagement
					err = c.crdClient.Put().Resource(crdv1.NodeFenceResourcePlural).Name(nf.Metadata.Name).Body(&nf).Do().Into(&nf)
					if err != nil {
						glog.Errorf("Failed to update nodefence': %s", err)
						return
					}

				}
			}
		}
	}
}

func (c *Controller) onNodeAdd(obj interface{}) {
	node := obj.(*v1.Node)
	nodeName := node.Name
	glog.V(4).Infof("add node: %s", nodeName)
	status := node.Status
	conditions := status.Conditions

	for _, condition := range conditions {
		if v1.NodeReady == condition.Type && v1.ConditionUnknown == condition.Status {
			glog.Warningf("Node %s ready status is unknown", nodeName)
			if len(c.nodePVMap[nodeName]) > 0 {
				glog.Warningf("PVs on node %s:", nodeName)
				for _, pv := range c.nodePVMap[nodeName] {
					glog.Warningf("\t%v:", pv.Name)
					c.createNewNodeFenceObject(node, pv)
				}
			} else {
				c.createNewNodeFenceObject(node, nil)
			}
		}
	}
}

func (c *Controller) onNodeUpdate(oldObj, newObj interface{}) {
	oldNode := oldObj.(*v1.Node)
	newNode := newObj.(*v1.Node)
	nodeName := oldNode.Name
	glog.V(4).Infof("update node: %s/%s", oldNode.Name, newNode.Name)
	newStatus := newNode.Status
	newConditions := newStatus.Conditions
	oldStatus := oldNode.Status
	oldConditions := oldStatus.Conditions

	for _, newCondition := range newConditions {
		found := false
		for _, oldCondition := range oldConditions {
			if newCondition.LastTransitionTime == oldCondition.LastTransitionTime {
				found = true
				break
			}
		}
		if !found {
			if v1.NodeReady == newCondition.Type && v1.ConditionUnknown == newCondition.Status {
				glog.Warningf("node %s Ready status is unknown", nodeName)
				if len(c.nodePVMap[nodeName]) > 0 {
					glog.Warningf("PVs on node %s:", nodeName)
					for _, pv := range c.nodePVMap[nodeName] {
						glog.Warningf("\t%v:", pv.Name)
						c.createNewNodeFenceObject(newNode, pv)
					}
				} else {
					c.createNewNodeFenceObject(newNode, nil)
				}
			}
		}
	}

}

func (c *Controller) onNodeDelete(obj interface{}) {
	node := obj.(*v1.Node)
	glog.Infof("delete node: %s", node.Name)
}

func (c *Controller) onPodAdd(obj interface{}) {
	pod := obj.(*v1.Pod)
	glog.V(4).Infof("add pod: %s", pod.Name)
	if c.podHasOwner(pod) {
		c.updateNodePV(pod, true)
	}
}

func (c *Controller) onPodUpdate(oldObj, newObj interface{}) {
	oldPod := oldObj.(*v1.Pod)
	newPod := newObj.(*v1.Pod)
	glog.V(4).Infof("update pod: %s/%s", oldPod.Name, newPod.Name)
	//FIXME: should evaluate PV status between old and new pod before update node pv map
	if c.podHasOwner(newPod) {
		c.updateNodePV(newPod, true)
	}
}

func (c *Controller) onPodDelete(obj interface{}) {
	pod := obj.(*v1.Pod)
	glog.V(4).Infof("delete pod: %s", pod.Name)
	if c.podHasOwner(pod) {
		c.updateNodePV(pod, false)
	}
}

func (c *Controller) updateNodePV(pod *v1.Pod, toAdd bool) {
	node := pod.Spec.NodeName
	if len(node) == 0 {
		return
	}
	podPrinted := false
	for _, vol := range pod.Spec.Volumes {
		if vol.VolumeSource.PersistentVolumeClaim != nil {
			if !podPrinted {
				glog.V(4).Infof("Pod: %s/%s", pod.Namespace, pod.Name)
				glog.V(4).Infof("\tnode: %s", node)
				podPrinted = true
			}
			pvcName := vol.VolumeSource.PersistentVolumeClaim.ClaimName
			glog.V(4).Infof("\tpvc: %v", pvcName)
			pvc, err := c.client.CoreV1().PersistentVolumeClaims(pod.Namespace).Get(pvcName, metav1.GetOptions{})
			if err == nil {
				pvName := pvc.Spec.VolumeName
				if len(pvName) != 0 {
					glog.V(4).Infof("\tpv: %v", pvName)
					pv, err := c.client.CoreV1().PersistentVolumes().Get(pvName, metav1.GetOptions{})
					if err == nil {
						c.updateNodePVMap(node, pv, toAdd)
					}
				}
			}
		}
	}
}

func (c *Controller) podHasOwner(pod *v1.Pod) bool {
	if len(pod.OwnerReferences) != 0 {
		for _, owner := range pod.OwnerReferences {
			if owner.BlockOwnerDeletion != nil {
				glog.V(4).Infof("pod %s has owner %s %s", pod.Name, owner.Kind, owner.Name)
				return true
			}
		}
	}
	return false
}

func (c *Controller) updateNodePVMap(node string, pv *v1.PersistentVolume, toAdd bool) {
	c.nodePVLock.Lock()
	defer c.nodePVLock.Unlock()
	for i, p := range c.nodePVMap[node] {
		if p != nil && pv != nil && p.Name == pv.Name {
			if toAdd {
				// already in the map, not to add
				return
			}
			c.nodePVMap[node][i] = c.nodePVMap[node][len(c.nodePVMap[node])-1]
			c.nodePVMap[node] = c.nodePVMap[node][:len(c.nodePVMap[node])-1]
			glog.V(6).Infof("node %s pv map: %v", node, c.nodePVMap[node])
			return
		}
	}
	c.nodePVMap[node] = append(c.nodePVMap[node], pv)
	glog.V(6).Infof("node %s pv map: %v", node, c.nodePVMap[node])
}

func (c *Controller) createNewNodeFenceObject(node *v1.Node, pv *v1.PersistentVolume) {
	nfName := fmt.Sprintf("node-fence-%s-%s", node.Name, uuid.NewUUID())

	nodeFencing := &crdv1.NodeFence{
		Metadata: metav1.ObjectMeta{
			Name: nfName,
		},
		CleanResources: true,
		Step:           crdv1.NodeFenceStepIsolation,
		NodeName:       node.Name,
		Status:         crdv1.NodeFenceConditionNew,
	}

	backoff := wait.Backoff{
		Duration: crdPostInitialDelay,
		Factor:   crdPostFactor,
		Steps:    crdPostSteps,
	}
	var result crdv1.NodeFence
	err := wait.ExponentialBackoff(backoff, func() (bool, error) {
		err := c.crdClient.Post().
			Resource(crdv1.NodeFenceResourcePlural).
			Body(nodeFencing).
			Do().Into(&result)
		if err != nil {
			// Re-Try it as errors writing to the API server are common
			return false, err
		}
		return true, nil
	})
	if err != nil {
		glog.Warningf("failed to post NodeFence CRD object: %v", err)
	} else {
		glog.Infof("posted NodeFence CRD object for node %s", node.Name)
	}
}
