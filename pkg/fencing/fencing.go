package fencing

import (
	"errors"
	"fmt"
	"github.com/golang/glog"
	crdv1 "github.com/rootfs/node-fencing/pkg/apis/crd/v1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"strings"
)

// GetNodeFenceConfig find the configmap obj relate to nodeName.
// configmap for nodes starts with "fence-config-" concat with nodeName.
// The function returns NodeFenceConfig filled with method lists for each fence step.
func GetNodeFenceConfig(nodeName string, c kubernetes.Interface) (crdv1.NodeFenceConfig, error) {
	fenceConfigName := "fence-config-" + nodeName
	nodeFields := GetConfigValues(fenceConfigName, "config.properties", c)
	if nodeFields == nil {
		return crdv1.NodeFenceConfig{}, errors.New("failed to read fence config for node")
	}
	config := crdv1.NodeFenceConfig{
		NodeName:        nodeName,
		Isolation:       strings.Split(nodeFields["isolation"], " "),
		PowerManagement: strings.Split(nodeFields["power_management"], " "),
		Recovery:        strings.Split(nodeFields["recovery"], " "),
	}
	return config, nil
}

// ExecuteFenceAgents gets NodeFenceConfig and the step to run.
// The function iterates over methods' names, fetch their parameters and
// executes the function related to the method
func ExecuteFenceAgents(config crdv1.NodeFenceConfig, step crdv1.NodeFenceStepType, c kubernetes.Interface) error {
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
		params := getMethodParams(config.NodeName, method, c)
		// find template if exists and add its fields
		if temp, exists := params["template"]; exists {
			tempParams := GetConfigValues(temp, "template.properties", c)
			for k, v := range tempParams {
				params[k] = v
			}
		}
		glog.Infof("ExecuteFenceAgents::Executing method: %s", method)
		node, err := c.CoreV1().Nodes().Get(config.NodeName, metav1.GetOptions{})
		if err != nil {
			glog.Errorf("ExecuteFenceAgents::Failed to get node: %s", err)
			return err
		}
		err = executeFence(params, node)
		if err != nil {
			return err
		}
	}
	glog.Infof("ExecuteFenceAgents::Finish execution for node: %s, step: %s", config.NodeName, step)
	return nil
}

// executeFence calls function related to agent_name in the method parameters
func executeFence(params map[string]string, node *v1.Node) error {
	if agentName, exists := params["agent_name"]; exists {
		if agent, exists := agents[agentName]; exists {
			if exists != true {
				return errors.New(fmt.Sprintf("executeFence::failed to find agent_name %s", agentName))
			}
			return agent.Function(params, node)
		}
	}
	return errors.New("executeFence::agent_name parameter does not exist in fence method configuration")
}

// getMethodParams returns map with the fence-method-[methodName]-[nodeName] parameters
func getMethodParams(nodeName string, methodName string, c kubernetes.Interface) map[string]string {
	methodFullName := "fence-method-" + methodName + "-" + nodeName
	return GetConfigValues(methodFullName, "method.properties", c)
}
