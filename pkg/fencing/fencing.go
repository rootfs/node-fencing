package fencing

import (
	"errors"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/golang/glog"
	crdv1 "github.com/rootfs/node-fencing/pkg/apis/crd/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var agents = make(map[string]crdv1.Agent)

func init() {
	// register agents
	agents["ssh"] = crdv1.Agent{
		Name:     "ssh",
		Desc:     "agent login to host via ssh and restart kubelet",
		Function: sshFenceAgentFunc,
	}
	agents["fence_eaton_snmp"] = crdv1.Agent{
		Name:     "fence_eaton_snmp",
		Desc:     "fence eaton agent to perform power management operations",
		Function: eatonSNMPAgentFunc,
	}
}

func sshFenceAgentFunc(params map[string]string) error {
	cmd := exec.Command("/usr/bin/k8s_node_fencing.sh", params["Address"])
	WaitTimeout(cmd, 1000)
	output, err := cmd.CombinedOutput()
	glog.Infof("fencing output: %s", string(output))
	if err == nil {
		glog.Infof("fencing node %s succeeded", params["nodeName"])
		return nil
	}
	return err
}

func eatonSNMPAgentFunc(_ map[string]string) error {
	// run command "/usr/bin/fence_eaton_snmp" with node.Status.Addresses.Address
	glog.Warning("EatonSNMPAgentFunc is not implemented yet")
	return nil
}

func getNodeFenceConfig(nodeName string, c kubernetes.Interface) (crdv1.NodeFenceConfig, error) {
	nodename := nodeName
	fenceConfigName := "fence-config-" + nodename
	nodeFields := getConfigValues(fenceConfigName, "config.properties", c)
	if nodeFields == nil {
		return crdv1.NodeFenceConfig{}, errors.New("failed to read fence config for node")
	}
	config := crdv1.NodeFenceConfig{
		NodeName:        nodeName,
		Isolation:       strings.Split(nodeFields["isolation"], " "),
		PowerManagement: strings.Split(nodeFields["power_management"], " "),
		Recovery:        strings.Split(nodeFields["Recovery"], " "),
	}
	return config, nil
}

// ExecuteFenceAgents gets fenceconfig and step, parse it, and calls executeFence
func ExecuteFenceAgents(config crdv1.NodeFenceConfig, step crdv1.NodeFenceStepType, c kubernetes.Interface) {
	methods := []string{}

	switch step {
	case crdv1.NodeFenceStepIsolation:
		methods = config.Isolation
	case crdv1.NodeFenceStepPowerManagement:
		methods = config.PowerManagement
	case crdv1.NodeFenceStepRecovery:
		methods = config.PowerManagement
	default:
		glog.Errorf("step is invalid %s", step)
	}

	for _, method := range methods {
		params := getMethodParams(config.NodeName, method, c)
		// find template if exists and add its params also
		if temp, exists := params["template"]; exists {
			tempParams := getConfigValues(temp, "template.properties", c)
			for k, v := range tempParams {
				params[k] = v
			}
		}
		err := executeFence(params)
		if err != nil {
			glog.Errorf("failed to execute agent %s", err)
		}
	}
}

func executeFence(params map[string]string) error {
	if agentName, exists := params["agent_name"]; exists {
		if agent, exists := agents[agentName]; exists {
			if exists != true {
				glog.Infof("Failed to find agent: %s", agentName)
				return errors.New("failed to find agent")
			}
			err := agent.Function(params)
			if err != nil {
				glog.Infof("fencing node failed: %s", err)
				return err
			}
			// success
			return nil
		}
	}
	return errors.New("agent name does not exist in fence parameters")
}

func getMethodParams(nodeName string, methodName string, c kubernetes.Interface) map[string]string {
	methodFullName := "fence-method-" + methodName + "-" + nodeName
	return getConfigValues(methodFullName, "method.properties", c)
}

func getConfigValues(configName string, configType string, c kubernetes.Interface) map[string]string {
	config, err := c.CoreV1().ConfigMaps("default").Get(configName, metav1.GetOptions{})
	if err != nil {
		glog.Errorf("failed to get %s", err)
		return nil
	}
	properties, _ := config.Data[configType]
	fields := make(map[string]string)

	for _, prop := range strings.Split(properties, "\n") {
		param := strings.Split(prop, "=")
		if len(param) == 2 {
			fields[param[0]] = param[1]
		}
	}
	return fields
}

// WaitTimeout waits for cmd to exist, if time pass pass sigkill signal
func WaitTimeout(cmd *exec.Cmd, timeout time.Duration) (err error) {
	ch := make(chan error)
	go func() {
		ch <- cmd.Wait()
	}()
	timer := time.NewTimer(timeout)
	select {
	case <-timer.C:
		cmd.Process.Signal(syscall.SIGKILL)
		close(ch)
		return errors.New("execute timeout")
	case err = <-ch:
		timer.Stop()
		return err
	}
}
