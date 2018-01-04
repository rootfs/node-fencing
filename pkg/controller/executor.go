package controller

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/golang/glog"
	crdv1 "github.com/rootfs/node-fencing/pkg/apis/crd/v1"
	"github.com/rootfs/node-fencing/pkg/fencing"
	batchv1 "k8s.io/api/batch/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	jobImageName = "docker.io/bronhaim/agent-image:latest"
)

// Executor object implements watcher functionality for nodefence objects
type Executor struct {
	crdClient           *rest.RESTClient
	crdScheme           *runtime.Scheme
	client              kubernetes.Interface
	runningJobs         *batchv1.JobList
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
	// TODO: use configurable namespace name
	err := c.initRunningJobs("default")
	if err != nil {
		glog.Errorf("couldn't initialized running job")
		os.Exit(1)
	}
	c.handleExistingNodeFences()
}

func (c *Executor) initRunningJobs(namespace string) error {
	runningJobsTemp, err := c.client.BatchV1().Jobs(namespace).List(metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("could not load running job: %s", err)
	}
	c.runningJobs = runningJobsTemp
	return nil
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
	err = c.executeFenceAgents(config, nf.Step)
	if err != nil {
		glog.Errorf("Failed to execute fence - moving to ERROR: %s", err)
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

// ExecuteFenceAgents gets NodeFenceConfig and the step to run.
// The function iterates over methods' names, fetch their parameters and
// executes the function related to the method
func (c *Executor) executeFenceAgents(config crdv1.NodeFenceConfig, step crdv1.NodeFenceStepType) error {
	glog.Infof("Running fence execution for node %s, step %s", config.NodeName, step)
	methods := []string{}

	switch step {
	case crdv1.NodeFenceStepIsolation:
		methods = config.Isolation
	case crdv1.NodeFenceStepPowerManagement:
		methods = config.PowerManagement
	case crdv1.NodeFenceStepRecovery:
		methods = config.Recovery
	default:
		return errors.New("ExecuteFenceAgents::Invalid step parameter")
	}

	for _, method := range methods {
		if method == "" {
			glog.Infof("ExecuteFenceAgents::Nothing to execute in step %s", step)
			return nil
		}
		params := fencing.GetMethodParams(config.NodeName, method, c.client)
		// find template if exists and add its fields
		if temp, exists := params["template"]; exists {
			tempParams := fencing.GetConfigValues(temp, "template.properties", c.client)
			for k, v := range tempParams {
				params[k] = v
			}
		}
		glog.Infof("ExecuteFenceAgents::Executing method: %s", method)
		node, err := c.client.CoreV1().Nodes().Get(config.NodeName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("ExecuteFenceAgents::Failed to get node: %s", err)
		}
		err = c.runFence(params, node)
		if err != nil {
			return err
		}
	}
	glog.Infof("ExecuteFenceAgents::Finish execution for node: %s, step: %s", config.NodeName, step)
	return nil
}

// runFence calls function related to agent_name in the method parameters
func (c *Executor) runFence(params map[string]string, node *apiv1.Node) error {
	if agentName, exists := params["agent_name"]; exists {
		if agent, exists := fencing.Agents[agentName]; exists {
			if exists != true {
				return fmt.Errorf("executeFence::failed to find agent_name %s", agentName)
			}
			// commet out old mechanism which uses exec.Command - remove when jobs handling works well
			// return agent.Function(params, node)

			job, err := c.postNewJobObj(
				// TODO: use configuratble namespace
				"default",
				agent.ExtractParameters(params, node),
				params["agent_name"],
				jobImageName)
			if err != nil {
				return fmt.Errorf("executeFence::failed to create job: %s", err)
			}
			c.runningJobs.Items = append(c.runningJobs.Items, *job)
			return nil
		}
	}
	return errors.New("executeFence::agent_name parameter does not exist in fence method configuration")
}

func (c *Executor) postNewJobObj(namespace string, cmd []string, name string, image string) (*batchv1.Job, error) {
	job := new(batchv1.Job)
	container := apiv1.Container{
		Name:    name,
		Image:   image,
		Command: cmd,
	}

	job.Name = name
	job.TypeMeta = metav1.TypeMeta{}
	job.ObjectMeta = metav1.ObjectMeta{Name: name}
	job.Spec = batchv1.JobSpec{
		Template: apiv1.PodTemplateSpec{
			Spec: apiv1.PodSpec{
				RestartPolicy: "Never",
				Containers:    []apiv1.Container{container},
			}},
	}
	job, err := c.client.BatchV1().Jobs(namespace).Create(job)
	return job, err
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
