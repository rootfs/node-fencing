package fencing

import (
	"encoding/xml"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/golang/glog"
	apiv1 "k8s.io/api/core/v1"
)

type agentParameterType int

const (
	agentParameterTypeBoolean agentParameterType = iota
	agentParameterTypeString
	agentParameterTypeInteger
)

type AgentParameter struct {
	// Parameter is required
	Required bool
	/*
	 * Parameter is deprecated by parameter with name DeprecatedBy.
	 * Parameter with name DeprecatedBy has Obsoletes filled to current parameter name
	 */
	Deprecated    bool
	DeprecatedBy  string
	Obsoletes     string
	ParameterType agentParameterType
}

// Agent defined the name, description and function for specific fence function
type Agent struct {
	Name              string
	Desc              string
	ExecutablePath    string
	Parameters        map[string]AgentParameter
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

	if fenceAgentExtractXMLFromMatchPath("/usr/sbin/fence_*", true, Agents) != nil {
		glog.Warningf("Can't load fence agents from given path")
	}

	// Explicitly define some agents (going to be removed soon)
	Agents["ssh"] = Agent{
		Name:              "ssh",
		Desc:              "Agent login to host via ssh and restart kubelet - requires copy-id first to allow root login",
		ExtractParameters: sshFenceAgentExtractParams,
	}
	Agents["gcloud-reset-inst"] = Agent{
		Name:              "google-cloud",
		Desc:              "Reboot instance in GCE cluster",
		ExtractParameters: gceAgentFuncExtractParam,
	}
	Agents["cordon"] = Agent{
		Name:              "cordon",
		Desc:              "Stop scheduler from using resources on node",
		ExtractParameters: runShellScriptExtractParam,
	}
	Agents["uncordon"] = Agent{
		Name:              "uncordon",
		Desc:              "Remove cordon from node",
		ExtractParameters: runShellScriptExtractParam,
	}
	Agents["clean-pods"] = Agent{
		Name:              "clean-pods",
		Desc:              "Delete all pod objects that runs on node_name",
		ExtractParameters: runCleanPodsExtractParam,
	}
}

func runShellScriptWithNodeName(params map[string]string, node *apiv1.Node) error {
	cmd := exec.Command("/bin/sh", params["script_path"], node.Name)
	return waitExec(cmd)
}

func runCleanPodsExtractParam(params map[string]string, node *apiv1.Node) []string {
	var ret []string
	ret = append(ret, fmt.Sprintf("--resource=%s", node.Name))
	ret = append(ret, fmt.Sprintf("--namespace=%s", params["namespace"]))
	return ret
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

func fenceAgentGetXML(agentPath string) ([]byte, error) {
	cmd := exec.Command(agentPath, "--action", "metadata")

	return cmd.CombinedOutput()
}

/*
 * Parse clusterlabs fencing agent XML get by running fenceAgentGetXML.
 * If addDeprecatedOptions is set, deprecated options are added into result Agent structure.
 * Number of parameter types is (for now) reduced to boolean (no parameter value required),
 * integer (ether "integer" or "second" type) and string (all other types including "select")
 */
func fenceAgentExtractXML(agentPath string, addDeprecatedOptions bool) (Agent, error) {
	type fenceAgentXMLParameterContent struct {
		Type         string `xml:"type,attr"`
		DefaultValue string `xml:"default,attr"`
	}

	type fenceAgentXMLParameter struct {
		Name       string                        `xml:"name,attr"`
		Required   int                           `xml:"required,attr"`
		Deprecated int                           `xml:"deprecated,attr"`
		Obsoletes  string                        `xml:"obsoletes,attr"`
		Content    fenceAgentXMLParameterContent `xml:"content"`
	}

	type resourceAgentXMLParameters struct {
		XMLName          xml.Name                 `xml:"resource-agent"`
		AgentName        string                   `xml:"name,attr"`
		AgentDescription string                   `xml:"shortdesc,attr"`
		Parameters       []fenceAgentXMLParameter `xml:"parameters>parameter"`
	}

	agentXML, err := fenceAgentGetXML(agentPath)
	if err != nil {
		return Agent{}, err
	}

	xmlParameters := resourceAgentXMLParameters{}

	err = xml.Unmarshal(agentXML, &xmlParameters)
	if err != nil {
		return Agent{}, err
	}

	resultAgent := Agent{}

	agentName := strings.Replace(xmlParameters.AgentName, "_", "-", -1)

	resultAgent.Name = agentName
	resultAgent.Desc = xmlParameters.AgentDescription
	resultAgent.ExecutablePath = agentPath
	resultAgent.Parameters = make(map[string]AgentParameter)
	resultAgent.ExtractParameters = fenceAgentExtractParams

	for _, parameter := range xmlParameters.Parameters {
		deprecated := parameter.Deprecated != 0

		if deprecated && !addDeprecatedOptions {
			continue
		}

		parameterName := strings.Replace(parameter.Name, "_", "-", -1)

		resultAgentParameter := AgentParameter{}
		resultAgentParameter.Required = (parameter.Required != 0)
		resultAgentParameter.Deprecated = deprecated

		obsoletes := strings.Replace(parameter.Obsoletes, "_", "-", -1)
		resultAgentParameter.Obsoletes = obsoletes

		switch parameter.Content.Type {
		case "string":
			resultAgentParameter.ParameterType = agentParameterTypeString
		case "select":
			resultAgentParameter.ParameterType = agentParameterTypeString
		case "integer":
			resultAgentParameter.ParameterType = agentParameterTypeInteger
		case "second":
			resultAgentParameter.ParameterType = agentParameterTypeInteger
		case "boolean":
			resultAgentParameter.ParameterType = agentParameterTypeBoolean
		default:
			resultAgentParameter.ParameterType = agentParameterTypeString
		}

		resultAgent.Parameters[parameterName] = resultAgentParameter
	}

	for parameterName, parameter := range resultAgent.Parameters {
		if parameter.Obsoletes != "" {
			obsoleted := resultAgent.Parameters[parameter.Obsoletes]
			obsoleted.DeprecatedBy = parameterName
			resultAgent.Parameters[parameter.Obsoletes] = obsoleted
		}
	}

	return resultAgent, nil
}

func fenceAgentExtractXMLFromMatchPath(matchPath string, addDeprecatedOptions bool, agents map[string]Agent) error {
	agentFiles, err := filepath.Glob(matchPath)
	if err != nil {
		return err
	}

	for _, agentFile := range agentFiles {
		glog.Infof("Extracting XML for agent %s", agentFile)

		resultAgent, err := fenceAgentExtractXML(agentFile, addDeprecatedOptions)
		if err != nil {
			glog.Warningf("Can't parse agent %s XML", agentFile)
		}
		agents[resultAgent.Name] = resultAgent
	}

	return nil
}

func fenceAgentParseBoolString(s string) (bool, error) {
	var res bool

	if strings.EqualFold(s, "on") || strings.EqualFold(s, "yes") ||
		strings.EqualFold(s, "true") || strings.EqualFold(s, "1") {
		res = true
	} else if strings.EqualFold(s, "off") || strings.EqualFold(s, "no") ||
		strings.EqualFold(s, "false") || strings.EqualFold(s, "0") {
		res = false
	} else {
		return false, fmt.Errorf("Unknown boolean value %s", s)
	}

	return res, nil
}

/*
 * TODO: Change definition to return error
 *       Check all required parameters are entered
 */
func fenceAgentExtractParams(params map[string]string, _ *apiv1.Node) []string {
	var ret []string

	if _, exists := params["agent_name"]; !exists {
		glog.Errorf("Agent name is not set in the parameters")

		return ret
	}

	agentName := params["agent_name"]

	if _, exists := Agents[agentName]; !exists {
		glog.Errorf("Agent with name %s doesn't exists", agentName)

		return ret
	}

	agent := Agents[agentName]

	ret = append(ret, agent.ExecutablePath)

	for paramName, paramValue := range params {
		if paramName == "agent_name" {
			continue
		}

		if _, exists := agent.Parameters[paramName]; !exists {
			glog.Warningf("Passing unknown parameter %s to agent %s. Parameter ignored",
				paramName, agentName)
			continue
		}

		agentParameter := agent.Parameters[paramName]

		switch agentParameter.ParameterType {
		case agentParameterTypeString:
			ret = append(ret, fmt.Sprintf("--%s=%s", paramName, paramValue))
		case agentParameterTypeInteger:
			ret = append(ret, fmt.Sprintf("--%s=%s", paramName, paramValue))
		case agentParameterTypeBoolean:
			appendParameter, err := fenceAgentParseBoolString(paramValue)
			if err != nil {
				glog.Warning(err)
				return []string{}
			}

			if appendParameter {
				ret = append(ret, fmt.Sprintf("--%s", paramName))
			}
		}
	}

	return ret
}
