package fencing

import (
	"os/exec"
	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	crdv1 "github.com/rootfs/node-fencing/pkg/apis/crd/v1"
	"k8s.io/client-go/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
	"errors"
	"time"
	"syscall"
)

var agents map[string]crdv1.Agent

func init() {
	// register agents
	agents["ssh"] = crdv1.Agent{Name: "ssh", Desc: "", Parameters: nil, Command: "/usr/bin/k8s_node_fencing.sh"}
	agents["fence_eaton_snmp"] = crdv1.Agent{Name: "fence_eaton_snmp", Desc: "", Parameters: nil, Command: "/usr/bin/fence_eaton_snmp"}
	// ...
}

func GetNodeFenceConfig(node *v1.Node, c kubernetes.Interface) crdv1.NodeFenceConfig {
	nodename := node.Name
	fence_config_name := "fence-config-" + nodename
	node_fields := getConfigValues(fence_config_name, "config.properties", c)

	config := crdv1.NodeFenceConfig{
		Node: *node,
		Isolation: strings.Split(node_fields["isolation"], " "),
		PowerManagement: strings.Split(node_fields["power_management"], " "),
		Recovery: strings.Split(node_fields["Recovery"], " "),
	}
	return config
}

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
		params := getMethodParams(config.Node.Name, method, c)
		// find template if exists and add its params also
		if temp, exists := params["template"]; exists {
			temp_params := getConfigValues(temp, "template.properties", c)
			for k, v := range temp_params {
				params[k] = v
			}
		}
		if agent_name, exists := params["agent_name"]; exists {
			glog.Infof("executing agent %s with params!", agent_name)

			err := ExecuteFence(&config.Node, agent_name)
			if err != nil {
				glog.Errorf("failed to execute agent %s", err)
			}

		} else {
			glog.Errorf("agent name field is missing from method config %s", method)
		}
	}
}

func ExecuteFence(node *v1.Node, agent_name string) error {
	if agent, exists := agents[agent_name]; exists {
		if exists != true {
			glog.Infof("Failed to find agent: %s", agent_name)
			return errors.New("failed to find agent")
		} else {
			addresses := node.Status.Addresses
			for _, addr := range addresses {
				if v1.NodeInternalIP == addr.Type || v1.NodeExternalIP == addr.Type {
					cmd := exec.Command(agent.Command, addr.Address)
					WaitTimeout(cmd, 1000)
					output, err := cmd.CombinedOutput()
					glog.Infof("fencing output: %s", string(output))
					if err == nil {
						glog.Infof("fencing node %s succeeded", node.Name)
					}
					glog.Infof("fencing to %s failed:%v", addr.Address, err)
					return err
				}
			}

		}
	}
	return nil
}

func getMethodParams(node_name string, method_name string, c kubernetes.Interface) map[string]string {
	method_full_name := "fence-method-config-" + node_name + method_name
	return getConfigValues(method_full_name, "method.properties", c)
}

func getConfigValues(config_name string, config_type string, c kubernetes.Interface) map[string]string {
	config, err := c.CoreV1().ConfigMaps("default").Get(config_name, metav1.GetOptions{})
	if err != nil {
		glog.Errorf("failed to get configmap %s", err)
	}
	properties, _ := config.Data[config_type]
	fields := make(map[string]string)

	for _, prop := range strings.Split(properties, "\n") {
		param := strings.Split(prop, "=")
		fields[param[0]] = param[1]
	}
	return fields
}

// Wait for cmd to exist, if time pass pass sigkill signal
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