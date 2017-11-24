package fencing

import (
	"os/exec"
	"github.com/golang/glog"
	//"github.com/rootfs/node-fencing/pkg/fencing/providers"
	"k8s.io/api/core/v1"
	crdv1 "github.com/rootfs/node-fencing/pkg/apis/crd/v1"
	"k8s.io/client-go/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

const (
	defaultFencingScript = "/usr/bin/k8s_node_fencing.sh"
)

func sshFencing(node *v1.Node) {
	addresses := node.Status.Addresses
	for _, addr := range addresses {
		if v1.NodeInternalIP == addr.Type || v1.NodeExternalIP == addr.Type {
			cmd := exec.Command(defaultFencingScript, addr.Address)
			output, err := cmd.CombinedOutput()
			glog.Infof("fencing output: %s", string(output))
			if err == nil {
				glog.Infof("fencing succeeded")
				return
			}
			glog.Infof("fencing to %s failed:%v", addr.Address, err)
		}
	}
}

func Fencing(node *v1.Node, pv *v1.PersistentVolume) {
	if pv.Spec.FC != nil ||
		pv.Spec.ISCSI != nil ||
		pv.Spec.RBD != nil ||
		pv.Spec.NFS != nil ||
		pv.Spec.Glusterfs != nil ||
		pv.Spec.CephFS != nil {
		sshFencing(node)
	}
}

func GetNodeFenceConfig(node *v1.Node, c kubernetes.Interface) crdv1.NodeFenceConfig {
	nodename := node.Name
	fence_config_name := "fence-config-" + nodename
	node_fields := getConfigValues(fence_config_name, "config.properties", c)

	config := crdv1.NodeFenceConfig{
		NodeName: node_fields["node_name"],
		Isolation: strings.Split(node_fields["isolation"], " "),
		PowerManagement: strings.Split(node_fields["power_management"], " "),
		Recovery: strings.Split(node_fields["Recovery"], " "),
	}
	return config
}

func ExecuteFenceAgents(config crdv1.NodeFenceConfig, step string, c kubernetes.Interface) {
	methods := []string{}

	switch step {
	case "isolation":
		methods = config.Isolation
	case "power-management":
		methods = config.PowerManagement
	case "recover":
		methods = config.PowerManagement
	default:
		glog.Errorf("step is invalid %s", step)
	}

	for _, method := range methods {
		params := getMethodParams(config.NodeName, method, c)
		// find template if exists and add its params also
		if temp, exists := params["template"]; exists {
			temp_params := getConfigValues(temp, "template.properties", c)
			for k, v := range temp_params {
				params[k] = v
			}
		}
		if agent_name, exists := params["agent_name"]; exists {
			glog.Infof("executing agent %s with params!", agent_name)

			// TODO - use providers


		} else {
			glog.Errorf("agent name field is missing from method config %s", method)
		}
	}
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
