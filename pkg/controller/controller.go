package controller

import (
	"os"
	"time"

	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
)

type Controller struct {
	client         kubernetes.Interface
	volumeMap      map[string][]string
	nodeController cache.Controller
	podController  cache.Controller
}

func NewNodeFencingController(client kubernetes.Interface) *Controller {
	volMap := make(map[string][]string)
	c := &Controller{
		client:    client,
		volumeMap: volMap,
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
	glog.Infof("node controller starting")
	go c.nodeController.Run(ctx)
	glog.Infof("pod controller starting")
	go c.podController.Run(ctx)
	glog.Infof("Waiting for informer initial sync")
	wait.Poll(time.Second, 5*time.Minute, func() (bool, error) {
		return c.nodeController.HasSynced() && c.podController.HasSynced(), nil
	})
	if !c.nodeController.HasSynced() {
		glog.Errorf("node informer controller initial sync timeout")
		os.Exit(1)
	}
	if !c.podController.HasSynced() {
		glog.Errorf("pod informer controller initial sync timeout")
		os.Exit(1)
	}
}

func (c *Controller) onNodeAdd(obj interface{}) {
	node := obj.(*v1.Node)
	glog.Infof("add node: %s", node.Name)
}

func (c *Controller) onNodeUpdate(oldObj, newObj interface{}) {
	oldNode := oldObj.(*v1.Node)
	newNode := newObj.(*v1.Node)
	glog.Infof("update node: %s/%s", oldNode.Name, newNode.Name)
}

func (c *Controller) onNodeDelete(obj interface{}) {
	node := obj.(*v1.Node)
	glog.Infof("delete node: %s", node.Name)
}

func (c *Controller) onPodAdd(obj interface{}) {
	pod := obj.(*v1.Pod)
	glog.Infof("add Pod: %s/%s", pod.Namespace, pod.Name)
}

func (c *Controller) onPodUpdate(oldObj, newObj interface{}) {
	oldPod := oldObj.(*v1.Pod)
	newPod := newObj.(*v1.Pod)
	glog.Infof("update pod: %s/%s", oldPod.Name, newPod.Name)
}

func (c *Controller) onPodDelete(obj interface{}) {
	node := obj.(*v1.Pod)
	glog.Infof("delete pod: %s", node.Name)
}
