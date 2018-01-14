package controller

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"

	crdv1 "github.com/rootfs/node-fencing/pkg/apis/crd/v1"

	apiv1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	"strconv"

	"github.com/rootfs/node-fencing/pkg/fencing"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/uuid"
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

	jobImageName     = "docker.io/bronhaim/agent-image:latest"
	workingNamespace = "default"
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

	nodePVMap       map[string][]*apiv1.PersistentVolume
	nodePVLock      *sync.Mutex
	nodeController  cache.Controller
	podController   cache.Controller
	eventController cache.Controller
}

// NewNodeFencingController initializing controller
func NewNodeFencingController(client kubernetes.Interface, crdClient *rest.RESTClient, crdScheme *runtime.Scheme) *Controller {
	c := &Controller{
		client:     client,
		nodePVMap:  make(map[string][]*apiv1.PersistentVolume),
		nodePVLock: &sync.Mutex{},
		crdClient:  crdClient,
		crdScheme:  crdScheme,
	}

	nodeListWatcher := cache.NewListWatchFromClient(
		client.Core().RESTClient(),
		"nodes",
		apiv1.NamespaceAll,
		fields.Everything())

	_, nodeController := cache.NewInformer(
		nodeListWatcher,
		&apiv1.Node{},
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
		apiv1.NamespaceAll,
		fields.Everything())

	_, podController := cache.NewInformer(
		podListWatcher,
		&apiv1.Pod{},
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
		apiv1.NamespaceAll,
		fields.Everything())

	_, eventController := cache.NewInformer(
		eventListWatcher,
		&apiv1.Event{},
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
	glog.Infof("handleExistingNodeFences::Controller monitor is running every %d seconds", elapsedPeriod)
	for range time.Tick(time.Duration(elapsedPeriod * time.Second)) {
		if !c.forceClusterPolices(roles) {
			continue
		}
		var nodeFences crdv1.NodeFenceList
		var giveup bool

		err := c.crdClient.Get().Resource(crdv1.NodeFenceResourcePlural).Do().Into(&nodeFences)
		if err != nil {
			glog.Errorf("handleExistingNodeFences::could not fetch nodefences - %s", err)
		} else {
			for _, nf := range nodeFences.Items {
				switch nf.Status {
				case crdv1.NodeFenceConditionNew:
					c.handleNodeFenceNew(nf)
				case crdv1.NodeFenceConditionError:
					c.handleNodeFenceError(nf)
				case crdv1.NodeFenceConditionRunning:
					// set failOnError to true based on retries
					giveup = false
					if nf.Retries == giveupRetries {
						giveup = true
					}
					c.handleNodeFenceRunning(nf, giveup)
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

// handleNodeFenceNew triggered when nodefence on status new
func (c *Controller) handleNodeFenceNew(nf crdv1.NodeFence) {
	c.startExecution(nf)
}

// handleNodeFenceError this function is called when nodefence object on status Error,
// in this phase we clean all related job and change nodefence status to New to retrigger
// all jobs.
func (c *Controller) handleNodeFenceError(nf crdv1.NodeFence) {
	// TODO: before cleaning job check which node ran them and set affinity
	c.cleanAllNodeFenceJobsList(nf)
	glog.Infof("handleNodeFenceError::Fence handling retries for node %s failed.", nf.NodeName)

	nf.Retries = 0
	nf.Status = crdv1.NodeFenceConditionNew
	c.updateNodefenceObj(nf)
}

// updateNodefenceObj helper function for updating nodefence obj
func (c *Controller) updateNodefenceObj(nf crdv1.NodeFence) {
	err := c.crdClient.Put().Resource(crdv1.NodeFenceResourcePlural).Name(nf.Metadata.Name).Body(&nf).Do().Into(&nf)
	if err != nil {
		glog.Errorf("Failed to update nodefence status: %s", err)
		return
	}
}

// handleNodeFenceRunning this function is called when nodefence object on status Running,
// in this phase we read all related job objects by their names, and check if they on completed
// status, if all related jobs completed move nodefence to Done state and clean jobs,
// if not, if failOnError is true - set nodefence status to Error
func (c *Controller) handleNodeFenceRunning(nf crdv1.NodeFence, failOnError bool) {
	done := true
	for _, jobName := range nf.Jobs {
		jobObj, err := c.client.BatchV1().Jobs(workingNamespace).Get(jobName, metav1.GetOptions{})
		if err != nil {
			glog.Errorf("Failed to get job object: %s", err)
		}
		if !fencing.CheckJobComplition(*jobObj) {
			done = false
			break
		}
	}
	if done {
		nf.Status = crdv1.NodeFenceConditionDone
		c.updateNodefenceObj(nf)
		c.cleanAllNodeFenceJobsList(nf)
	} else {
		if failOnError {
			nf.Status = crdv1.NodeFenceConditionError
			c.updateNodefenceObj(nf)
			// Q: clean old jobs or leave to them on fail state?
		} else {
			nf.Retries = nf.Retries + 1
			c.updateNodefenceObj(nf)
		}
	}
}

// cleanAllNodeFenceJobsList this function goes over all jobs and delete their object
func (c *Controller) cleanAllNodeFenceJobsList(nf crdv1.NodeFence) {
	for _, jobName := range nf.Jobs {
		err := c.client.BatchV1().Jobs(workingNamespace).Delete(jobName, &metav1.DeleteOptions{})
		if err != nil {
			glog.Errorf("Failed to delete job object: %s", err)
			return
		}
	}
}

// handleNodeFenceDone when nodefence on status Done, this function will update the step.
// if step is Recovery, nodefence is removed
// if node back to readiness - move to Recovery
// if still not ready - move to power-manegement
// if already in PM state and giveup_retries passed - move to Error
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
	var node *apiv1.Node
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
		nf.Retries = 0
		nf.Step = crdv1.NodeFenceStepRecovery
		c.updateNodefenceObj(nf)
	} else {
		switch nf.Step {
		case crdv1.NodeFenceStepIsolation:
			glog.Infof("Isolation is done - moving to Power-Management step for node %s", nf.NodeName)
			nf.Status = crdv1.NodeFenceConditionNew
			nf.Retries = 0
			nf.Step = crdv1.NodeFenceStepPowerManagement
			c.updateNodefenceObj(nf)
		case crdv1.NodeFenceStepPowerManagement:
			// TODO: treat if doesn't come back for giveup_retries
			glog.Infof("Node %s after PM operations - waiting for status change", nf.NodeName)
		}
	}
}

func (c *Controller) onNodeAdd(obj interface{}) {
	node := obj.(*apiv1.Node)
	glog.V(4).Infof("add node: %s", node.Name)

	for _, condition := range node.Status.Conditions {
		if !c.checkReadiness(node, condition) {
			c.createNewNodeFenceObject(node, nil)
		}
	}
}

func (c *Controller) checkReadiness(node *apiv1.Node, cond apiv1.NodeCondition) bool {
	readiness := true
	nodeName := node.Name
	if apiv1.NodeReady == cond.Type && apiv1.ConditionUnknown == cond.Status {
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
	oldNode := oldObj.(*apiv1.Node)
	newNode := newObj.(*apiv1.Node)
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
	node := obj.(*apiv1.Node)
	glog.Infof("delete node: %s", node.Name)
}

func (c *Controller) onPodAdd(obj interface{}) {
	pod := obj.(*apiv1.Pod)
	glog.V(4).Infof("add pod: %s", pod.Name)
	if c.podHasOwner(pod) {
		c.updateNodePV(pod, true)
	}
}

func (c *Controller) onPodUpdate(oldObj, newObj interface{}) {
	oldPod := oldObj.(*apiv1.Pod)
	newPod := newObj.(*apiv1.Pod)
	glog.V(4).Infof("update pod: %s/%s", oldPod.Name, newPod.Name)
	//FIXME: should evaluate PV status between old and new pod before update node pv map
	if c.podHasOwner(newPod) {
		c.updateNodePV(newPod, true)
	}
}

func (c *Controller) onPodDelete(obj interface{}) {
	pod := obj.(*apiv1.Pod)
	glog.V(4).Infof("delete pod: %s", pod.Name)
	if c.podHasOwner(pod) {
		c.updateNodePV(pod, false)
	}
}

func (c *Controller) onEventAdd(obj interface{}) {
	event := obj.(*apiv1.Event)
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

func (c *Controller) updateNodePV(pod *apiv1.Pod, toAdd bool) {
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

func (c *Controller) podHasOwner(pod *apiv1.Pod) bool {
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

func (c *Controller) updateNodePVMap(node string, pv *apiv1.PersistentVolume, toAdd bool) {
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

func (c *Controller) createNewNodeFenceObject(node *apiv1.Node, pv *apiv1.PersistentVolume) {
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
		Retries:  0,
		Step:     crdv1.NodeFenceStepIsolation,
		NodeName: node.Name,
		Status:   crdv1.NodeFenceConditionNew,
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
		glog.Infof("Posted NodeFence CRD object for node %s", node.Name)
	}
}

func (c *Controller) nodeProblemEvent(event *apiv1.Event) (bool, string) {
	if event == nil {
		return false, ""
	}
	if event.Type == apiv1.EventTypeWarning &&
		supportedNodeProblemSources.Has(string(event.Source.Component)) {
		return true, event.Source.Host
	}
	return false, ""
}

// startExecution gets nodefence obj, retrieve required fields to run ExecuteFenceAgents and updates
// the nodefence obj based on the return value
func (c *Controller) startExecution(nf crdv1.NodeFence) {
	config, err := fencing.GetNodeFenceConfig(nf.NodeName, c.client)
	if err != nil {
		glog.Errorf("Node fencing failed on node %s", nf.NodeName)
		return
	}
	jobsNames, err := c.executeFenceAgents(config, nf.Step)
	if err != nil {
		glog.Errorf("Failed to execute fence - moving to ERROR: %s", err)
		nf.Status = crdv1.NodeFenceConditionError
		err = c.crdClient.Put().Resource(crdv1.NodeFenceResourcePlural).Name(nf.Metadata.Name).Body(&nf).Do().Into(&nf)
		if err != nil {
			glog.Errorf("Failed to update status to 'running': %s", err)
			return
		}
	} else {
		nf.Status = crdv1.NodeFenceConditionRunning
		nf.Jobs = jobsNames
		err = c.crdClient.Put().Resource(crdv1.NodeFenceResourcePlural).Name(nf.Metadata.Name).Body(&nf).Do().Into(&nf)
		if err != nil {
			glog.Errorf("Failed to update status to 'running': %s", err)
			return
		}
	}
}

// executeFenceAgents gets NodeFenceConfig and the step to run.
// The function iterates over methods' names, fetch their parameters and
// executes job related to the method. There run go function to monitor the jobs till all finish
// succesfully.
func (c *Controller) executeFenceAgents(config crdv1.NodeFenceConfig, step crdv1.NodeFenceStepType) ([]string, error) {
	glog.Infof("Running fence execution for node %s, step %s", config.NodeName, step)
	methods := []string{}
	jobsNamesList := []string{}

	switch step {
	case crdv1.NodeFenceStepIsolation:
		methods = config.Isolation
	case crdv1.NodeFenceStepPowerManagement:
		methods = config.PowerManagement
	case crdv1.NodeFenceStepRecovery:
		methods = config.Recovery
	default:
		return jobsNamesList, errors.New("ExecuteFenceAgents::Invalid step parameter")
	}

	for _, method := range methods {
		if method == "" {
			glog.Infof("ExecuteFenceAgents::Nothing to execute in step %s", step)
			return nil, nil
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
			return jobsNamesList, fmt.Errorf("ExecuteFenceAgents::Failed to get node: %s", err)
		}
		jobName, err := c.runFence(params, node)
		if err != nil {
			return jobsNamesList, err
		}
		jobsNamesList = append(jobsNamesList, jobName)
	}
	return jobsNamesList, nil
}

// runFence calls function related to agent_name in the method parameters
func (c *Controller) runFence(params map[string]string, node *apiv1.Node) (string, error) {
	if agentName, exists := params["agent_name"]; exists {
		if agent, exists := fencing.Agents[agentName]; exists {
			job, err := c.postNewJobObj(
				workingNamespace,
				agent.ExtractParameters(params, node),
				params["agent_name"],
				jobImageName)
			if err != nil {
				return "", fmt.Errorf("executeFence::failed to create job: %s", err)
			}
			glog.Infof("New job created - %s", job.Name)
			return job.Name, nil
		}
		return "", fmt.Errorf("executeFence::%s agent is missing", params["agent_name"])
	}
	return "", errors.New("executeFence::agent_name parameter does not exist in fence method configuration")
}

func (c *Controller) postNewJobObj(namespace string, cmd []string, name string, image string) (*batchv1.Job, error) {
	job := new(batchv1.Job)
	container := apiv1.Container{
		Name:    name,
		Image:   image,
		Command: cmd,
	}

	jobUniqueName := fmt.Sprintf("%s-%s", name, uuid.NewUUID())
	job.Name = jobUniqueName
	job.TypeMeta = metav1.TypeMeta{}
	job.ObjectMeta = metav1.ObjectMeta{Name: jobUniqueName}
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
