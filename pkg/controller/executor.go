package controller

import (
	"os"
	"time"

	"github.com/golang/glog"
	crdv1 "github.com/rootfs/node-fencing/pkg/apis/crd/v1"
	"github.com/rootfs/node-fencing/pkg/fencing"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
)

type Executor struct {
	crdClient           *rest.RESTClient
	crdScheme           *runtime.Scheme
	client              kubernetes.Interface
	nodeFenceController cache.Controller
}

// NewNodeFenceExecutorController initializing executor controller with nodeFenceController that listens on nodefence
// CRD changes
func NewNodeFenceExecutorController(client kubernetes.Interface, crdClient *rest.RESTClient, crdScheme *runtime.Scheme) *Executor {
	c := &Executor{
		client:    client,
		crdClient: crdClient,
		crdScheme: crdScheme,
	}

	// Watch NodeFence objects
	eventListWatcher := cache.NewListWatchFromClient(
		c.crdClient,
		crdv1.NodeFenceResourcePlural,
		apiv1.NamespaceAll,
		fields.Everything())

	_, nodeFenceController := cache.NewInformer(
		eventListWatcher,
		&crdv1.NodeFence{},
		time.Minute*60,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.onNodeFencingAdd,
			UpdateFunc: c.onNodeFencingUpdate,
		},
	)

	c.nodeFenceController = nodeFenceController

	return c
}

// Run starts watcher and handling existing nodefence obj on start - to avoid waiting for updates
func (c *Executor) Run(ctx <-chan struct{}) {
	glog.Infof("Node fence executor starting")
	go c.nodeFenceController.Run(ctx)
	glog.Infof("Waiting for informer initial sync")
	wait.Poll(time.Second, 5*time.Minute, func() (bool, error) {
		return c.nodeFenceController.HasSynced(), nil
	})
	if !c.nodeFenceController.HasSynced() {
		glog.Errorf("node fence informer controller initial sync timeout")
		os.Exit(1)
	}
	c.handleExistingNodeFences()
}

// handleExistingNodeFences goes over all nodefences object and check if should start execution
func (c *Executor) handleExistingNodeFences() {
	var nodeFences crdv1.NodeFenceList
	glog.Infof("Handling existing node fences ... ")
	err := c.crdClient.Get().Resource(crdv1.NodeFenceResourcePlural).Do().Into(&nodeFences)
	if err != nil {
		glog.Errorf("something went wrong - could not fetch nodefences - %s", err)
	} else {
		for _, nf := range nodeFences.Items {
			glog.Infof("Read %s ..", nf.Metadata.Name)
			switch nf.Status {
			case crdv1.NodeFenceConditionNew:
				glog.Infof("nodefence in NEW state")
				c.startExecution(nf)
			case crdv1.NodeFenceConditionError:
				// If this host already tried to handle nodefence object report an error and let
				// controller handle it
				hostname, _ := os.Hostname()
				if nf.Hostname == hostname {
					glog.Infof("handleExistingNodeFences::nodefence in ERROR state")
				}
			case crdv1.NodeFenceConditionRunning:
				glog.Infof("Node fence is already Running on: %s", nf.Hostname)
			}
		}
	}
}

// startExecution gets nodefence obj, retrieve required fields to run ExecuteFenceAgents and updates
// the nodefence obj based on the return value
func (c *Executor) startExecution(nf crdv1.NodeFence) {
	config, err := fencing.GetNodeFenceConfig(nf.NodeName, c.client)
	if err != nil {
		glog.Errorf("Node fencing failed on node %s", nf.NodeName)
		return
	}
	nf.Status = crdv1.NodeFenceConditionRunning
	hostname, _ := os.Hostname()
	nf.Hostname = hostname
	err = c.crdClient.Put().Resource(crdv1.NodeFenceResourcePlural).Name(nf.Metadata.Name).Body(&nf).Do().Into(&nf)
	if err != nil {
		glog.Errorf("Failed to update status to 'running': %s", err)
		return
	}
	err = fencing.ExecuteFenceAgents(config, nf.Step, c.client)
	if err != nil {
		glog.Errorf("Failed to execute fence - moving to ERROR:", err)
		nf.Status = crdv1.NodeFenceConditionError
	} else {
		nf.Status = crdv1.NodeFenceConditionDone
	}
	err = c.crdClient.Put().Resource(crdv1.NodeFenceResourcePlural).Name(nf.Metadata.Name).Body(&nf).Do().Into(&nf)
	if err != nil {
		glog.Errorf("Failed to update status to 'running': %s", err)
		return
	}
}

func (c *Executor) onNodeFencingAdd(obj interface{}) {
	fence := obj.(*crdv1.NodeFence)
	glog.Infof("New fence object %s", fence.Metadata.Name)
	c.startExecution(*fence)
}

func (c *Executor) onNodeFencingUpdate(oldObj, newObj interface{}) {
	oldFence := oldObj.(*crdv1.NodeFence)
	newFence := newObj.(*crdv1.NodeFence)
	if oldFence.Step != newFence.Step {
		c.startExecution(*newFence)
	}
}
