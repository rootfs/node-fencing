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
	crdClient             *rest.RESTClient
	crdScheme             *runtime.Scheme
	client                kubernetes.Interface
	nodeFencingController cache.Controller
}

func NewNodeFencingExecutorController(client kubernetes.Interface, crdClient *rest.RESTClient, crdScheme *runtime.Scheme) *Executor {
	c := &Executor{
		client:    client,
		crdClient: crdClient,
		crdScheme: crdScheme,
	}

	// Watch NodeFence objects
	source := cache.NewListWatchFromClient(
		c.crdClient,
		crdv1.NodeFencingResourcePlural,
		apiv1.NamespaceAll,
		fields.Everything())

	_, nodeFencingController := cache.NewInformer(
		source,
		&crdv1.NodeFence{},
		time.Minute*60,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.onNodeFencingAdd,
			UpdateFunc: c.onNodeFencingUpdate,
			DeleteFunc: c.onNodeFencingDelete,
		},
	)

	c.nodeFencingController = nodeFencingController

	return c
}

func (c *Executor) Run(ctx <-chan struct{}) {
	glog.Infof("node Fencing executor starting")
	go c.nodeFencingController.Run(ctx)
	glog.Infof("Waiting for informer initial sync")
	wait.Poll(time.Second, 5*time.Minute, func() (bool, error) {
		return c.nodeFencingController.HasSynced(), nil
	})
	if !c.nodeFencingController.HasSynced() {
		glog.Errorf("node fencing informer controller initial sync timeout")
		os.Exit(1)
	}
	glog.Infof("Watching node fencing object")
}

func (c *Executor) onNodeFencingAdd(obj interface{}) {
	fence := obj.(*crdv1.NodeFence)
	fencing.ExecuteFenceAgents(fencing.GetNodeFenceConfig(&fence.Node, c.client), fence.Step, c.client)
}

func (c *Executor) onNodeFencingUpdate(_, _ interface{}) {
}

func (c *Executor) onNodeFencingDelete(_ interface{}) {
}
