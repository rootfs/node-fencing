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
	agents["gcloud-reset-inst"] = crdv1.Agent{
		Name:     "google-cloud",
		Desc:     "Reboot instance in GCE cluster",
		Function: gceAgentFunc,
	}

	agents["cordon"] = crdv1.Agent{
		Name:     "cordon",
		Desc:     "Stop scheduler from using resources on node",
		Function: cordonFunc,
	}
	agents["uncordon"] = crdv1.Agent{
		Name:     "uncordon",
		Desc:     "Remove cordon from node",
		Function: uncordonFunc,
	}
}

func cordonFunc(params map[string]string, node *v1.Node) error {
	cmd := exec.Command("/bin/sh",
		"fence-scripts/k8s_cordon_node.sh",
		node.Name)
	return waitExec(cmd)
}

func uncordonFunc(params map[string]string, node *v1.Node) error {
	cmd := exec.Command("/bin/sh",
		"fence-scripts/k8s_uncordon_node.sh",
		node.Name)
	return waitExec(cmd)
}

func gceAgentFunc(params map[string]string, node *v1.Node) error {
	cmd := exec.Command("/usr/bin/python",
		"fence-scripts/k8s_gce_reboot_instance.py",
		node.Name)
	return waitExec(cmd)
}

func waitExec(cmd *exec.Cmd) error {
	WaitTimeout(cmd, 3000)
	output, err := cmd.CombinedOutput()
	glog.Infof("Agent output: %s", string(output))
	return err
}

func sshFenceAgentFunc(params map[string]string, node *v1.Node) error {
	add := node.Status.Addresses[0].Address
	cmd := exec.Command("/bin/sh", "fence-scripts/k8s_ssh_fence.sh", add)
	return waitExec(cmd)
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
	return waitExec(cmd)
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
		Recovery:        strings.Split(nodeFields["recovery"], " "),
	}
	return config, nil
}

// ExecuteFenceAgents gets fenceconfig and step, parse it, and calls executeFence
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
		glog.Errorf("step is invalid %s", step)
		return errors.New("invalid step parameter")
	}

	for _, method := range methods {
		if method == "" {
			glog.Infof("Nothing to execute in step %s", step)
			return nil
		}
		params := getMethodParams(config.NodeName, method, c)
		// find template if exists and add its fields
		if temp, exists := params["template"]; exists {
			tempParams := getConfigValues(temp, "template.properties", c)
			for k, v := range tempParams {
				params[k] = v
			}
		}
		glog.Infof("Executing method: %s", method)
		node, err := c.CoreV1().Nodes().Get(config.NodeName, metav1.GetOptions{})
		if err != nil {
			glog.Errorf("Failed to get node: %s", err)
			return err
		}
		return executeFence(params, node)
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
			return agent.Function(params, node)
		}
	}
	return errors.New("configured agent_name does not exist.")
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
