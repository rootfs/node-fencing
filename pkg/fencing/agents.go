package fencing

import (
	"fmt"
	"os/exec"

	"github.com/golang/glog"
	apiv1 "k8s.io/api/core/v1"
)

// Agent defined the name, description and function for specific fence function
type Agent struct {
	Name              string
	Desc              string
	Function          func(params map[string]string, node *apiv1.Node) error
	ExtractParameters func(params map[string]string, node *apiv1.Node) []string
}

// Agents map holds Agent structs. key is the agent_name param in fence method configmap
// e.g. for the following configmap agent_name is gcloud-reset-inst
// In agents map we should have entry for "gcloud-reset-inst". In Agent.Function value
// we set pointer to the implementation (gceAgentFunc)
//- kind: ConfigMap
//  apiVersion: v1
//  metadata:
//  	name: fence-method-gcloud-reset-inst-kubernetes-minion-group-9ssp
//  	namespace: default
//	data:
//		method.properties: |
//			agent_name=gcloud-reset-inst
//			zone=us-east1-b
//			project=kube-cluster-fence-poc
var Agents = make(map[string]Agent)

func init() {
	// Register agents
	// For each agent_name we define description and function pointer for the execution logic

	// TODO: make dynamic load from folder /usr/libexec/fence-agents
	// filename will be the key, and function only executes the scripts with parameters from the the configmaps

	// For now - we explicitly define Agent structure for each script under fence-scripts folder
	Agents["ssh"] = Agent{
		Name:              "ssh",
		Desc:              "Agent login to host via ssh and restart kubelet - requires copy-id first to allow root login",
		Function:          sshFenceAgentFunc,
		ExtractParameters: sshFenceAgentExtractParams,
	}
	Agents["fence_apc_snmp"] = Agent{
		Name:              "fence_apc_snmp",
		Desc:              "Fence agent for APC, Tripplite PDU over SNMP",
		Function:          apcSNMPAgentFunc,
		ExtractParameters: apcSNMPAgentExtractParams,
	}
	Agents["gcloud-reset-inst"] = Agent{
		Name:              "google-cloud",
		Desc:              "Reboot instance in GCE cluster",
		Function:          gceAgentFunc,
		ExtractParameters: gceAgentFuncExtractParam,
	}
	Agents["cordon"] = Agent{
		Name:              "cordon",
		Desc:              "Stop scheduler from using resources on node",
		Function:          runShellScriptWithNodeName,
		ExtractParameters: runShellScriptExtractParam,
	}
	Agents["uncordon"] = Agent{
		Name:              "uncordon",
		Desc:              "Remove cordon from node",
		Function:          runShellScriptWithNodeName,
		ExtractParameters: runShellScriptExtractParam,
	}
	Agents["clean-pods"] = Agent{
		Name:              "clean-pods",
		Desc:              "Delete all pod objects that runs on node_name",
		Function:          runShellScriptWithNodeName,
		ExtractParameters: runShellScriptExtractParam,
	}
}

func runShellScriptWithNodeName(params map[string]string, node *apiv1.Node) error {
	cmd := exec.Command("/bin/sh", params["script_path"], node.Name)
	return waitExec(cmd)
}
func runShellScriptExtractParam(params map[string]string, node *apiv1.Node) []string {
	var ret []string
	ret = append(ret, "/bin/sh")
	ret = append(ret, params["script_path"])
	ret = append(ret, node.Name)
	return ret
}

func gceAgentFunc(params map[string]string, node *apiv1.Node) error {
	// can use reflect here and pass extractParam string list to command
	cmd := exec.Command("/usr/bin/python",
		"fence-scripts/k8s_gce_reboot_instance.py",
		node.Name)
	return waitExec(cmd)
}
func gceAgentFuncExtractParam(params map[string]string, node *apiv1.Node) []string {
	var ret []string
	ret = append(ret, "/usr/bin/python")
	ret = append(ret, "/usr/sbin/k8s_gce_reboot_instance.sh")
	ret = append(ret, node.Name)
	return ret
}

func waitExec(cmd *exec.Cmd) error {
	WaitTimeout(cmd, 3000)
	output, err := cmd.CombinedOutput()
	glog.Infof("Agent output: %s", string(output))
	return err
}

func sshFenceAgentFunc(params map[string]string, node *apiv1.Node) error {
	add := node.Status.Addresses[0].Address
	cmd := exec.Command("/bin/sh", "fence-scripts/k8s_ssh_fence.sh", add)
	return waitExec(cmd)
}

func sshFenceAgentExtractParams(params map[string]string, node *apiv1.Node) []string {
	var ret []string
	ret = append(ret, "/bin/sh")
	ret = append(ret, "/usr/sbin/k8s_ssh_fence.sh")
	ret = append(ret, node.Status.Addresses[0].Address)
	return ret
}

func apcSNMPAgentFunc(params map[string]string, _ *apiv1.Node) error {
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

func apcSNMPAgentExtractParams(params map[string]string, _ *apiv1.Node) []string {
	var ret []string
	ret = append(ret, "/usr/bin/python")
	ret = append(ret, "/usr/sbin/fence_apc_snmp")
	ret = append(ret, fmt.Sprintf("--ip=%s", params["address"]))
	ret = append(ret, fmt.Sprintf("--username=%s", params["username"]))
	ret = append(ret, fmt.Sprintf("--password=%s", params["password"]))
	ret = append(ret, fmt.Sprintf("--plug=%s", params["plug"]))
	ret = append(ret, fmt.Sprintf("--action=%s", params["action"]))
	return ret
}
