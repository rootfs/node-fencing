package fencing

import (
	"errors"
	"strings"

	crdv1 "github.com/rootfs/node-fencing/pkg/apis/crd/v1"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/client-go/kubernetes"
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

// GetMethodParams returns map with the fence-method-[methodName]-[nodeName] parameters
func GetMethodParams(nodeName string, methodName string, c kubernetes.Interface) map[string]string {
	methodFullName := "fence-method-" + methodName + "-" + nodeName
	return GetConfigValues(methodFullName, "method.properties", c)
}

// CheckJobComplition helper func to check job condition field, if one
// condition status type is JobComplete return true
func CheckJobComplition(job batchv1.Job) bool {
	for _, cond := range job.Status.Conditions {
		if cond.Type == batchv1.JobComplete {
			return true
		}
	}
	return false
}
