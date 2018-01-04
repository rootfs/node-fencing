package controller

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"

	crdv1 "github.com/rootfs/node-fencing/pkg/apis/crd/v1"

	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	"strconv"

	"github.com/rootfs/node-fencing/pkg/fencing"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	crdPostInitialDelay = 2 * time.Second
	crdPostFactor       = 1.2
	crdPostSteps        = 5
	// following defaults are used if fence-cluster-config configmap does not exists
	gracePeriodDefault = 5
	giveupRetries      = 5
	clusterPolicies    = ""
)

var (
	// TODO read supported source from node problem detector config
	supportedNodeProblemSources = sets.NewString("abrt-notification", "abrt-adaptor", "docker-monitor", "kernel-monitor", "kernel")
)

// Controller object implements watcher functionality for pods, nodes and events objects
type Controller struct {
	crdClient *rest.RESTClient
	crdScheme *runtime.Scheme
	client    kubernetes.Interface

	nodePVMap       map[string][]*v1.PersistentVolume
	nodePVLock      *sync.Mutex
	nodeController  cache.Controller
	podController   cache.Controller
	eventController cache.Controller
}

// NewNodeFencingController initializing controller
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

	eventListWatcher := cache.NewListWatchFromClient(
		client.Core().RESTClient(),
		"events",
		v1.NamespaceAll,
		fields.Everything())

	_, eventController := cache.NewInformer(
		eventListWatcher,
		&v1.Event{},
		time.Minute*60,
		cache.ResourceEventHandlerFuncs{
			AddFunc: c.onEventAdd,
		},
	)

	c.eventController = eventController

	return c
}

// Run starts watchers, start main loop and read cluster config
func (c *Controller) Run(ctx <-chan struct{}) {
	glog.Infof("Fence controller starting")

	go c.podController.Run(ctx)
	glog.Infof("Waiting for pod informer initial sync")
	wait.Poll(time.Second, 5*time.Minute, func() (bool, error) {
		return c.podController.HasSynced(), nil
	})
	if !c.podController.HasSynced() {
		glog.Errorf("pod informer initial sync timeout")
		os.Exit(1)
	}

	go c.nodeController.Run(ctx)
	glog.Infof("Waiting for node informer initial sync")
	wait.Poll(time.Second, 5*time.Minute, func() (bool, error) {
		return c.nodeController.HasSynced(), nil
	})

	if !c.nodeController.HasSynced() {
		glog.Errorf("node informer initial sync timeout")
		os.Exit(1)
	}

	go c.eventController.Run(ctx)
	glog.Infof("Waiting for event informer initial sync")
	wait.Poll(time.Second, 5*time.Minute, func() (bool, error) {
		return c.eventController.HasSynced(), nil
	})
	if !c.eventController.HasSynced() {
		glog.Errorf("event informer initial sync timeout")
		os.Exit(1)
	}

	// Reading fence-cluster-config - this sets roles and timeouts for controller job
	config := fencing.GetConfigValues("fence-cluster-config", "config.properties", c.client)
	graceTimeout := time.Duration(gracePeriodDefault)
	giveup := giveupRetries
	policies := clusterPolicies
	if config != nil {
		// using defaults
		gt, err := strconv.Atoi(config["grace_timeout"])
		if err != nil {
			glog.Errorf("grace_timeout in fence-cluster-config is not int")
		}
		graceTimeout = time.Duration(gt)

		gu, err := strconv.Atoi(config["giveup_retries"])
		if err != nil {
			glog.Errorf("giveup_retries in fence-cluster-config is not int")
		}
		giveup = gu

	}
	go c.handleExistingNodeFences(graceTimeout, giveup, strings.Split(policies, " "))
}

// handleExistingNodeFences go over nodefence objs every elapsedPeriod and
// modify the nodefence object based on its state
func (c *Controller) handleExistingNodeFences(elapsedPeriod time.Duration, giveupRetries int, roles []string) {
	glog.Infof("Controller monitor is running every %d seconds", elapsedPeriod)
	for range time.Tick(time.Duration(elapsedPeriod * time.Second)) {
		if !c.forceClusterPolices(roles) {
			continue
		}
		var nodeFences crdv1.NodeFenceList
		err := c.crdClient.Get().Resource(crdv1.NodeFenceResourcePlural).Do().Into(&nodeFences)
		if err != nil {
			glog.Errorf("something went wrong - could not fetch nodefences - %s", err)
		} else {
			for _, nf := range nodeFences.Items {
				switch nf.Status {
				case crdv1.NodeFenceConditionNew:
					c.handleNodeFenceNew(nf)
				case crdv1.NodeFenceConditionError:
					c.handleNodeFenceError(nf)
				case crdv1.NodeFenceConditionRunning:
					c.handleNodeFenceRunning(nf)
				case crdv1.NodeFenceConditionDone:
					c.handleNodeFenceDone(nf)
				}
			}
		}
	}
}

// forceClusterPolices validates policies based on parameters and cluster status.
// return false if fencing loop should not be proceeded
func (c *Controller) forceClusterPolices(policies []string) bool {

	//for _, policyName := range policies {
	//	config := fencing.GetConfigValues("fence-policy-" + roleName, "config.properties", c.client)
	//	switch policyName {
	//	case "fence-limit":
	//		// return false if more than config["precentage"] nodes are not responsive
	//		// this requires to fetch cluster status from cache
	//	}
	//
	//}

	return true
}

func (c *Controller) handleNodeFenceNew(_ crdv1.NodeFence) {
	// TODO: Check if one executor at least is running already - if not, start one
}

func (c *Controller) handleNodeFenceError(_ crdv1.NodeFence) {
	// TODO: Check if current executor failed to run - if so, initiates new executor
}

func (c *Controller) handleNodeFenceRunning(nf crdv1.NodeFence) {
	// currently we just write to log - nothing should occur if nodefence is already processing
	glog.Infof("Node fence is already running on: %s", nf.Hostname)
}

func (c *Controller) handleNodeFenceDone(nf crdv1.NodeFence) {
	if nf.Step == crdv1.NodeFenceStepRecovery { // recovery completed
		glog.Infof("Fence for node %s completed successfully", nf.NodeName)
		err := c.crdClient.Delete().Resource(crdv1.NodeFenceResourcePlural).Name(nf.Metadata.Name).Do().Into(&nf)
		if err != nil {
			glog.Errorf("Failed to delete nodefence': %s", err)
			return
		}
		return
	}
	// Check if node is still in unknown state
	var node *v1.Node
	var backToReady = true
	node, err := c.client.CoreV1().Nodes().Get(nf.NodeName, metav1.GetOptions{})
	if err != nil {
		glog.Errorf("Failed reading node obj: %s", nf.NodeName)
		return
	}
	for _, condition := range node.Status.Conditions {
		if !c.checkReadiness(node, condition) {
			backToReady = false
		}
	}
	if backToReady {
		glog.Infof("Node %s is back to ready state. Moving to Recovery stage", nf.NodeName)
		nf.Status = crdv1.NodeFenceConditionNew
		nf.Step = crdv1.NodeFenceStepRecovery
		err = c.crdClient.Put().Resource(crdv1.NodeFenceResourcePlural).Name(nf.Metadata.Name).Body(&nf).Do().Into(&nf)
		if err != nil {
			glog.Errorf("Failed to update nodefence': %s", err)
			return
		}
	} else {
		switch nf.Step {
		case crdv1.NodeFenceStepIsolation:
			glog.Infof("Isolation is done - moving to Power-Management step for node %s", nf.NodeName)
			nf.Status = crdv1.NodeFenceConditionNew
			nf.Step = crdv1.NodeFenceStepPowerManagement
			err = c.crdClient.Put().Resource(crdv1.NodeFenceResourcePlural).Name(nf.Metadata.Name).Body(&nf).Do().Into(&nf)
			if err != nil {
				glog.Errorf("Failed to update nodefence': %s", err)
				return
			}
		case crdv1.NodeFenceStepPowerManagement:
			// TODO: treat if doesn't come back for giveup_retries
			glog.Infof("Node %s after PM operations - waiting for status change", nf.NodeName)
		}
	}
}

func (c *Controller) onNodeAdd(obj interface{}) {
	node := obj.(*v1.Node)
	glog.V(4).Infof("add node: %s", node.Name)

	for _, condition := range node.Status.Conditions {
		if !c.checkReadiness(node, condition) {
			c.createNewNodeFenceObject(node, nil)
		}
	}
}

func (c *Controller) checkReadiness(node *v1.Node, cond v1.NodeCondition) bool {
	readiness := true
	nodeName := node.Name
	if v1.NodeReady == cond.Type && v1.ConditionUnknown == cond.Status {
		glog.Warningf("Node %s ready status is unknown", nodeName)
		if len(c.nodePVMap[nodeName]) > 0 {
			glog.Warningf("PVs on node %s:", nodeName)
			for _, pv := range c.nodePVMap[nodeName] {
				glog.Warningf("\t%v:", pv.Name)
				readiness = false
			}
		} else {
			readiness = false
		}
	}
	return readiness
}

func (c *Controller) onNodeUpdate(oldObj, newObj interface{}) {
	oldNode := oldObj.(*v1.Node)
	newNode := newObj.(*v1.Node)
	glog.V(4).Infof("update node: %s/%s", oldNode.Name, newNode.Name)

	for _, newCondition := range newNode.Status.Conditions {
		found := false
		for _, oldCondition := range oldNode.Status.Conditions {
			if newCondition.LastTransitionTime == oldCondition.LastTransitionTime {
				found = true
				break
			}
		}
		if !found {
			if !c.checkReadiness(newNode, newCondition) {
				c.createNewNodeFenceObject(newNode, nil)
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

func (c *Controller) onEventAdd(obj interface{}) {
	event := obj.(*v1.Event)
	glog.V(4).Infof("received: %v", event)
	// only process node problem event
	// TODO use rule based config to post fence object
	if problem, host := c.nodeProblemEvent(event); problem {
		glog.V(3).Infof("process node problem, node %s", host)
		node, err := c.client.CoreV1().Nodes().Get(host, metav1.GetOptions{})
		if err != nil {
			glog.Errorf("Failed to get node: %s", err)
		}
		c.createNewNodeFenceObject(node, nil)
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
	nfName := fmt.Sprintf("node-fence-%s", node.Name)

	var result crdv1.NodeFence
	err := c.crdClient.Get().Resource(crdv1.NodeFenceResourcePlural).Body(nfName).Do().Into(&result)
	// If no error means the resource already exists
	if err == nil {
		glog.Infof("nodefence CRD for node %s already exists", node.Name)
		return
	}

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

	err = wait.ExponentialBackoff(backoff, func() (bool, error) {
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
		glog.Infof("Posted NodeFence CRD object for node %s - starting Isolation", node.Name)
	}
}

func (c *Controller) nodeProblemEvent(event *v1.Event) (bool, string) {
	if event == nil {
		return false, ""
	}
	if event.Type == v1.EventTypeWarning &&
		supportedNodeProblemSources.Has(string(event.Source.Component)) {
		return true, event.Source.Host
	}
	return false, ""
}
