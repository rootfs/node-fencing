package fencing

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/golang/glog"
	crdv1 "github.com/rootfs/node-fencing/pkg/apis/crd/v1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var agents = make(map[string]crdv1.Agent)

func init() {
	// register agents
	agents["ssh"] = crdv1.Agent{
		Name:     "ssh",
		Desc:     "Agent login to host via ssh and restart kubelet - requires copy-id first to allow root login",
		Function: sshFenceAgentFunc,
	}
	agents["fence_apc_snmp"] = crdv1.Agent{
		Name:     "fence_apc_snmp",
		Desc:     "Fence agent for APC, Tripplite PDU over SNMP",
		Function: apcSNMPAgentFunc,
	}
	agents["google-cloud"] = crdv1.Agent{
		Name:     "google-cloud",
		Desc:     "Reboot instance in GCE cluster",
		Function: gceAgentFunc,
	}
}

func gceAgentFunc(params map[string]string, node *v1.Node) error {
	cmd := exec.Command("/usr/bin/python",
		"fence-scripts/k8s_instance_rest_fence.sh",
		node.Name)
	WaitTimeout(cmd, 1000)
	output, err := cmd.CombinedOutput()
	glog.Infof("fencing output: %s", string(output))
	if err != nil {
		glog.Infof("Fencing address: %s failed", params["address"])
		return err
	}
	return nil
}

func sshFenceAgentFunc(params map[string]string, node *v1.Node) error {
	add := node.Status.Addresses[0].Address
	cmd := exec.Command("/bin/sh", "fence-scripts/k8s_ssh_fence.sh", add)
	WaitTimeout(cmd, 1000)
	output, err := cmd.CombinedOutput()
	glog.Infof("fencing output: %s", string(output))
	if err != nil {
		glog.Infof("Fencing address: %s failed", add)
		return err
	}
	return nil
}

func apcSNMPAgentFunc(params map[string]string, _ *v1.Node) error {
	ip := fmt.Sprintf("--ip=%s", params["address"])
	username := fmt.Sprintf("--username=%s", params["username"])
	password := fmt.Sprintf("--password=%s", params["password"])
	plug := fmt.Sprintf("--plug=%s", params["plug"])
	action := fmt.Sprintf("--action=%s", params["action"])

	cmd := exec.Command(
		"/usr/bin/python",
		"/usr/sbin/fence_apc_snmp",
		ip,
		password,
		username,
		plug,
		action,
	)
	WaitTimeout(cmd, 1000)
	output, err := cmd.CombinedOutput()
	glog.Infof("fencing output: %s", string(output))
	if err != nil {
		glog.Infof("Fencing address: %s failed", params["address"])
		return err
	}
	return nil
}

// GetNodeFenceConfig find the nodefenceconfig obj relate to nodeName
func GetNodeFenceConfig(nodeName string, c kubernetes.Interface) (crdv1.NodeFenceConfig, error) {
	fenceConfigName := "fence-config-" + nodeName
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
func ExecuteFenceAgents(config crdv1.NodeFenceConfig, step crdv1.NodeFenceStepType, c kubernetes.Interface) error {
	glog.Infof("Start fence execution for node: %s", config.NodeName)
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
		return errors.New("invalid step parameter")
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
		glog.Infof("Executing method: %s", method)

		// need to fetch the address somehow :|
		node, err := c.CoreV1().Nodes().Get(config.NodeName, metav1.GetOptions{})
		if err != nil {
			glog.Errorf("Failed to get node: %s", err)
			return err
		}
		err = executeFence(params, node)
		if err != nil {
			glog.Errorf("Failed: %s", err)
			return err
		}
	}
	glog.Infof("Finish execution for node: %s", config.NodeName)
	return nil
}

func executeFence(params map[string]string, node *v1.Node) error {
	if agentName, exists := params["agent_name"]; exists {
		if agent, exists := agents[agentName]; exists {
			if exists != true {
				return errors.New("failed to find agent")
			}
			err := agent.Function(params, node)
			if err != nil {
				glog.Errorf("fencing node failed: %s", err)
				return err
			}
			// success
			return nil
		}
	}
	return errors.New("agent_name field does not exist in method config")
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
