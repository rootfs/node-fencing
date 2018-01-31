package fencing

import (
	"flag"
	"reflect"
	"sort"
	"testing"
)

/*
 * Metadata got running COS 7.4 fence_rhevm
 */
const fenceRhevmXMLMetadataCOS74 = `
<?xml version="1.0" ?>
<resource-agent name="fence_rhevm" shortdesc="Fence agent for RHEV-M REST API" >
<longdesc>fence_rhevm is an I/O Fencing agent which can be used with RHEV-M REST API to fence virtual machines.</longdesc>
<vendor-url>http://www.redhat.com</vendor-url>
<parameters>
	<parameter name="ipport" unique="0" required="0">
		<getopt mixed="-u, --ipport=[port]" />
		<content type="integer" default="80"  />
		<shortdesc lang="en">TCP/UDP port to use for connection with device</shortdesc>
	</parameter>
	<parameter name="notls" unique="0" required="0">
		<getopt mixed="-t, --notls" />
		<content type="boolean"  />
		<shortdesc lang="en">Disable TLS negotiation, force SSL 3.0</shortdesc>
	</parameter>
	<parameter name="ssl_secure" unique="0" required="0">
		<getopt mixed="--ssl-secure" />
		<content type="boolean"  />
		<shortdesc lang="en">SSL connection with verifying fence device's certificate</shortdesc>
	</parameter>
	<parameter name="port" unique="0" required="1" deprecated="1">
		<getopt mixed="-n, --plug=[id]" />
		<content type="string"  />
		<shortdesc lang="en">Physical plug number, name of virtual machine or UUID</shortdesc>
	</parameter>
	<parameter name="inet6_only" unique="0" required="0">
		<getopt mixed="-6, --inet6-only" />
		<content type="boolean"  />
		<shortdesc lang="en">Forces agent to use IPv6 addresses only</shortdesc>
	</parameter>
	<parameter name="ipaddr" unique="0" required="1" deprecated="1">
		<getopt mixed="-a, --ip=[ip]" />
		<content type="string"  />
		<shortdesc lang="en">IP Address or Hostname</shortdesc>
	</parameter>
	<parameter name="inet4_only" unique="0" required="0">
		<getopt mixed="-4, --inet4-only" />
		<content type="boolean"  />
		<shortdesc lang="en">Forces agent to use IPv4 addresses only</shortdesc>
	</parameter>
	<parameter name="passwd_script" unique="0" required="0" deprecated="1">
		<getopt mixed="-S, --password-script=[script]" />
		<content type="string"  />
		<shortdesc lang="en">Script to retrieve password</shortdesc>
	</parameter>
	<parameter name="passwd" unique="0" required="0" deprecated="1">
		<getopt mixed="-p, --password=[password]" />
		<content type="string"  />
		<shortdesc lang="en">Login password or passphrase</shortdesc>
	</parameter>
	<parameter name="ssl" unique="0" required="0">
		<getopt mixed="-z, --ssl" />
		<content type="boolean"  />
		<shortdesc lang="en">SSL connection</shortdesc>
	</parameter>
	<parameter name="use_cookies" unique="0" required="0">
		<getopt mixed="--use-cookies" />
		<content type="boolean"  />
		<shortdesc lang="en">Reuse cookies for authentication</shortdesc>
	</parameter>
	<parameter name="ssl_insecure" unique="0" required="0">
		<getopt mixed="--ssl-insecure" />
		<content type="boolean"  />
		<shortdesc lang="en">SSL connection without verifying fence device's certificate</shortdesc>
	</parameter>
	<parameter name="action" unique="0" required="1">
		<getopt mixed="-o, --action=[action]" />
		<content type="string" default="reboot"  />
		<shortdesc lang="en">Fencing Action</shortdesc>
	</parameter>
	<parameter name="login" unique="0" required="1" deprecated="1">
		<getopt mixed="-l, --username=[name]" />
		<content type="string"  />
		<shortdesc lang="en">Login Name</shortdesc>
	</parameter>
	<parameter name="plug" unique="0" required="1" obsoletes="port">
		<getopt mixed="-n, --plug=[id]" />
		<content type="string"  />
		<shortdesc lang="en">Physical plug number, name of virtual machine or UUID</shortdesc>
	</parameter>
	<parameter name="username" unique="0" required="1" obsoletes="login">
		<getopt mixed="-l, --username=[name]" />
		<content type="string"  />
		<shortdesc lang="en">Login Name</shortdesc>
	</parameter>
	<parameter name="ip" unique="0" required="1" obsoletes="ipaddr">
		<getopt mixed="-a, --ip=[ip]" />
		<content type="string"  />
		<shortdesc lang="en">IP Address or Hostname</shortdesc>
	</parameter>
	<parameter name="password" unique="0" required="0" obsoletes="passwd">
		<getopt mixed="-p, --password=[password]" />
		<content type="string"  />
		<shortdesc lang="en">Login password or passphrase</shortdesc>
	</parameter>
	<parameter name="password_script" unique="0" required="0" obsoletes="passwd_script">
		<getopt mixed="-S, --password-script=[script]" />
		<content type="string"  />
		<shortdesc lang="en">Script to retrieve password</shortdesc>
	</parameter>
	<parameter name="api_path" unique="0" required="0">
		<getopt mixed="--api-path=[path]" />
		<content type="string" default="/ovirt-engine/api"  />
		<shortdesc lang="en">The path of the API URL</shortdesc>
	</parameter>
	<parameter name="disable_http_filter" unique="0" required="0">
		<getopt mixed="--disable-http-filter" />
		<content type="boolean"  />
		<shortdesc lang="en">Set HTTP Filter header to false</shortdesc>
	</parameter>
	<parameter name="verbose" unique="0" required="0">
		<getopt mixed="-v, --verbose" />
		<content type="boolean"  />
		<shortdesc lang="en">Verbose mode</shortdesc>
	</parameter>
	<parameter name="debug" unique="0" required="0" deprecated="1">
		<getopt mixed="-D, --debug-file=[debugfile]" />
		<content type="string"  />
		<shortdesc lang="en">Write debug information to given file</shortdesc>
	</parameter>
	<parameter name="debug_file" unique="0" required="0" obsoletes="debug">
		<getopt mixed="-D, --debug-file=[debugfile]" />
		<content type="string"  />
		<shortdesc lang="en">Write debug information to given file</shortdesc>
	</parameter>
	<parameter name="version" unique="0" required="0">
		<getopt mixed="-V, --version" />
		<content type="boolean"  />
		<shortdesc lang="en">Display version information and exit</shortdesc>
	</parameter>
	<parameter name="help" unique="0" required="0">
		<getopt mixed="-h, --help" />
		<content type="boolean"  />
		<shortdesc lang="en">Display help and exit</shortdesc>
	</parameter>
	<parameter name="separator" unique="0" required="0">
		<getopt mixed="-C, --separator=[char]" />
		<content type="string" default=","  />
		<shortdesc lang="en">Separator for CSV created by operation list</shortdesc>
	</parameter>
	<parameter name="power_wait" unique="0" required="0">
		<getopt mixed="--power-wait=[seconds]" />
		<content type="second" default="1"  />
		<shortdesc lang="en">Wait X seconds after issuing ON/OFF</shortdesc>
	</parameter>
	<parameter name="login_timeout" unique="0" required="0">
		<getopt mixed="--login-timeout=[seconds]" />
		<content type="second" default="5"  />
		<shortdesc lang="en">Wait X seconds for cmd prompt after login</shortdesc>
	</parameter>
	<parameter name="power_timeout" unique="0" required="0">
		<getopt mixed="--power-timeout=[seconds]" />
		<content type="second" default="20"  />
		<shortdesc lang="en">Test X seconds for status change after ON/OFF</shortdesc>
	</parameter>
	<parameter name="delay" unique="0" required="0">
		<getopt mixed="--delay=[seconds]" />
		<content type="second" default="0"  />
		<shortdesc lang="en">Wait X seconds before fencing is started</shortdesc>
	</parameter>
	<parameter name="shell_timeout" unique="0" required="0">
		<getopt mixed="--shell-timeout=[seconds]" />
		<content type="second" default="3"  />
		<shortdesc lang="en">Wait X seconds for cmd prompt after issuing command</shortdesc>
	</parameter>
	<parameter name="retry_on" unique="0" required="0">
		<getopt mixed="--retry-on=[attempts]" />
		<content type="integer" default="1"  />
		<shortdesc lang="en">Count of attempts to retry power on</shortdesc>
	</parameter>
</parameters>
<actions>
	<action name="on" automatic="0"/>
	<action name="off" />
	<action name="reboot" />
	<action name="status" />
	<action name="list" />
	<action name="list-status" />
	<action name="monitor" />
	<action name="metadata" />
	<action name="validate-all" />
</actions>
</resource-agent>
`

const fenceRhevmXMLMetadataFC26 = `
<?xml version="1.0" ?>
<resource-agent name="fence_rhevm" shortdesc="Fence agent for RHEV-M REST API" >
<longdesc>fence_rhevm is an I/O Fencing agent which can be used with RHEV-M REST API to fence virtual machines.</longdesc>
<vendor-url>http://www.redhat.com</vendor-url>
<parameters>
	<parameter name="action" unique="0" required="1">
		<getopt mixed="-o, --action=[action]" />
		<content type="string" default="reboot"  />
		<shortdesc lang="en">Fencing action</shortdesc>
	</parameter>
	<parameter name="inet4_only" unique="0" required="0">
		<getopt mixed="-4, --inet4-only" />
		<content type="boolean"  />
		<shortdesc lang="en">Forces agent to use IPv4 addresses only</shortdesc>
	</parameter>
	<parameter name="inet6_only" unique="0" required="0">
		<getopt mixed="-6, --inet6-only" />
		<content type="boolean"  />
		<shortdesc lang="en">Forces agent to use IPv6 addresses only</shortdesc>
	</parameter>
	<parameter name="ipaddr" unique="0" required="1">
		<getopt mixed="-a, --ip=[ip]" />
		<content type="string"  />
		<shortdesc lang="en">IP address or hostname of fencing device</shortdesc>
	</parameter>
	<parameter name="ipport" unique="0" required="0">
		<getopt mixed="-u, --ipport=[port]" />
		<content type="string" default="80"  />
		<shortdesc lang="en">TCP/UDP port to use for connection with device</shortdesc>
	</parameter>
	<parameter name="login" unique="0" required="1">
		<getopt mixed="-l, --username=[name]" />
		<content type="string"  />
		<shortdesc lang="en">Login name</shortdesc>
	</parameter>
	<parameter name="notls" unique="0" required="0">
		<getopt mixed="-t, --notls" />
		<content type="boolean"  />
		<shortdesc lang="en">Disable TLS negotiation and force SSL3.0. This should only be used for devices that do not support TLS1.0 and up.</shortdesc>
	</parameter>
	<parameter name="passwd" unique="0" required="0">
		<getopt mixed="-p, --password=[password]" />
		<content type="string"  />
		<shortdesc lang="en">Login password or passphrase</shortdesc>
	</parameter>
	<parameter name="passwd_script" unique="0" required="0">
		<getopt mixed="-S, --password-script=[script]" />
		<content type="string"  />
		<shortdesc lang="en">Script to run to retrieve password</shortdesc>
	</parameter>
	<parameter name="port" unique="0" required="1">
		<getopt mixed="-n, --plug=[id]" />
		<content type="string"  />
		<shortdesc lang="en">Physical plug number on device, UUID or identification of machine</shortdesc>
	</parameter>
	<parameter name="ssl" unique="0" required="0">
		<getopt mixed="-z, --ssl" />
		<content type="boolean"  />
		<shortdesc lang="en">Use SSL connection with verifying certificate</shortdesc>
	</parameter>
	<parameter name="ssl_insecure" unique="0" required="0">
		<getopt mixed="--ssl-insecure" />
		<content type="boolean"  />
		<shortdesc lang="en">Use SSL connection without verifying certificate</shortdesc>
	</parameter>
	<parameter name="ssl_secure" unique="0" required="0">
		<getopt mixed="--ssl-secure" />
		<content type="boolean"  />
		<shortdesc lang="en">Use SSL connection with verifying certificate</shortdesc>
	</parameter>
	<parameter name="use_cookies" unique="0" required="0">
		<getopt mixed="--use-cookies" />
		<content type="boolean"  />
		<shortdesc lang="en">Reuse cookies for authentication</shortdesc>
	</parameter>
	<parameter name="verbose" unique="0" required="0">
		<getopt mixed="-v, --verbose" />
		<content type="boolean"  />
		<shortdesc lang="en">Verbose mode</shortdesc>
	</parameter>
	<parameter name="debug" unique="0" required="0">
		<getopt mixed="-D, --debug-file=[debugfile]" />
		<content type="string"  />
		<shortdesc lang="en">Write debug information to given file</shortdesc>
	</parameter>
	<parameter name="version" unique="0" required="0">
		<getopt mixed="-V, --version" />
		<content type="boolean"  />
		<shortdesc lang="en">Display version information and exit</shortdesc>
	</parameter>
	<parameter name="help" unique="0" required="0">
		<getopt mixed="-h, --help" />
		<content type="boolean"  />
		<shortdesc lang="en">Display help and exit</shortdesc>
	</parameter>
	<parameter name="separator" unique="0" required="0">
		<getopt mixed="-C, --separator=[char]" />
		<content type="string" default=","  />
		<shortdesc lang="en">Separator for CSV created by 'list' operation</shortdesc>
	</parameter>
	<parameter name="delay" unique="0" required="0">
		<getopt mixed="--delay=[seconds]" />
		<content type="string" default="0"  />
		<shortdesc lang="en">Wait X seconds before fencing is started</shortdesc>
	</parameter>
	<parameter name="login_timeout" unique="0" required="0">
		<getopt mixed="--login-timeout=[seconds]" />
		<content type="string" default="5"  />
		<shortdesc lang="en">Wait X seconds for cmd prompt after login</shortdesc>
	</parameter>
	<parameter name="power_timeout" unique="0" required="0">
		<getopt mixed="--power-timeout=[seconds]" />
		<content type="string" default="20"  />
		<shortdesc lang="en">Test X seconds for status change after ON/OFF</shortdesc>
	</parameter>
	<parameter name="power_wait" unique="0" required="0">
		<getopt mixed="--power-wait=[seconds]" />
		<content type="string" default="1"  />
		<shortdesc lang="en">Wait X seconds after issuing ON/OFF</shortdesc>
	</parameter>
	<parameter name="shell_timeout" unique="0" required="0">
		<getopt mixed="--shell-timeout=[seconds]" />
		<content type="string" default="3"  />
		<shortdesc lang="en">Wait X seconds for cmd prompt after issuing command</shortdesc>
	</parameter>
	<parameter name="retry_on" unique="0" required="0">
		<getopt mixed="--retry-on=[attempts]" />
		<content type="string" default="1"  />
		<shortdesc lang="en">Count of attempts to retry power on</shortdesc>
	</parameter>
	<parameter name="gnutlscli_path" unique="0" required="0">
		<getopt mixed="--gnutlscli-path=[path]" />
		<content type="string" default="/usr/bin/gnutls-cli"  />
		<shortdesc lang="en">Path to gnutls-cli binary</shortdesc>
	</parameter>
</parameters>
<actions>
	<action name="on" automatic="0"/>
	<action name="off" />
	<action name="reboot" />
	<action name="status" />
	<action name="list" />
	<action name="list-status" />
	<action name="monitor" />
	<action name="metadata" />
	<action name="validate-all" />
</actions>
</resource-agent>
`

func strMatch(str1, str2 string, t *testing.T) {
	if str1 != str2 {
		t.Error(str1, "!=", str2)
	}
}

func parameterMatch(agent Agent, param string, expectedParam AgentParameter, t *testing.T) {
	if _, exists := agent.Parameters[param]; !exists {
		t.Error("Parameter", param, "doesn't exist")
	}

	if !reflect.DeepEqual(agent.Parameters[param], expectedParam) {
		t.Error(param, ": given", agent.Parameters[param], "!= expected", expectedParam)
	}
}

func TestFenceAgentParseXMLCOS74(t *testing.T) {
	/*
	 * Allow glog log to stderr
	 */
	flag.Set("logtostderr", "true")
	flag.Parse()

	execPath := "/usr/sbin/fence_rhevm"

	agent, err := fenceAgentParseXML(execPath, []byte(fenceRhevmXMLMetadataCOS74))

	if err != nil {
		t.Error("Can't parse XML")
	} else {
		strMatch(agent.ExecutablePath, execPath, t)
		strMatch(agent.Name, "fence-rhevm", t)

		parameterMatch(agent, "ipport", AgentParameter{ParameterType: agentParameterTypeInteger, DefaultValue: "80"}, t)
		parameterMatch(agent, "plug", AgentParameter{Required: true, ParameterType: agentParameterTypeString}, t)
		parameterMatch(agent, "ssl-secure", AgentParameter{ParameterType: agentParameterTypeBoolean}, t)
		if _, exists := agent.Parameters["port"]; exists {
			t.Error("Parameter port exists but it shouldn't")
		}
		parameterMatch(agent, "ip", AgentParameter{Required: true, ParameterType: agentParameterTypeString}, t)
		if _, exists := agent.Parameters["ipaddr"]; exists {
			t.Error("Parameter ipaddr exists but it shouldn't")
		}
	}
}

func TestFenceAgentParseXMLFC26(t *testing.T) {
	/*
	 * Allow glog log to stderr
	 */
	flag.Set("logtostderr", "true")
	flag.Parse()

	execPath := "/usr/sbin/fence_rhevm"

	agent, err := fenceAgentParseXML(execPath, []byte(fenceRhevmXMLMetadataFC26))

	if err != nil {
		t.Error("Can't parse XML")
	} else {
		strMatch(agent.ExecutablePath, execPath, t)
		strMatch(agent.Name, "fence-rhevm", t)

		parameterMatch(agent, "ipport", AgentParameter{ParameterType: agentParameterTypeString, DefaultValue: "80"}, t)
		parameterMatch(agent, "plug", AgentParameter{Required: true, ParameterType: agentParameterTypeString}, t)
		parameterMatch(agent, "ssl-secure", AgentParameter{ParameterType: agentParameterTypeBoolean}, t)
		if _, exists := agent.Parameters["port"]; exists {
			t.Error("Parameter port exists but it shouldn't")
		}
		parameterMatch(agent, "ip", AgentParameter{Required: true, ParameterType: agentParameterTypeString}, t)
		if _, exists := agent.Parameters["ipaddr"]; exists {
			t.Error("Parameter ipaddr exists but it shouldn't")
		}
	}
}

func stringSliceMatch(slice1, slice2 []string) bool {
	if len(slice1) != len(slice2) {
		return false
	}

	slice1Copy := make([]string, len(slice1))
	copy(slice1Copy, slice1)
	sort.Strings(slice1Copy)

	slice2Copy := make([]string, len(slice2))
	copy(slice2Copy, slice2)
	sort.Strings(slice2Copy)

	for i := range slice1Copy {
		if slice1Copy[i] != slice2Copy[i] {
			return false
		}
	}

	return true
}

func extractParamsMatch(result []string, expectedResult []string, t *testing.T) {
	if !stringSliceMatch(result, expectedResult) {
		t.Error("given", result, "!= expected", expectedResult)
	}
}

func TestFenceAgentExtractParams(t *testing.T) {
	/*
	 * Allow glog log to stderr
	 */
	flag.Set("logtostderr", "true")
	flag.Parse()

	execPath := "/usr/sbin/fence_rhevm_test"

	resultAgent, err := fenceAgentParseXML(execPath, []byte(fenceRhevmXMLMetadataCOS74))
	if err != nil {
		t.Fatal("Can't parse XML")
	}

	if resultAgent.Name != "fence-rhevm" {
		t.Fatal("Unexpected resultAgent.Name")
	}

	Agents[resultAgent.Name] = resultAgent

	params := make(map[string]string)
	/*
	 * agent_name not defined
	 */
	result := fenceAgentExtractParams(params, nil)
	extractParamsMatch(result, []string{}, t)

	params["agent_name"] = resultAgent.Name
	result = fenceAgentExtractParams(params, nil)
	extractParamsMatch(result, []string{}, t)

	params["plug"] = "test_plug"
	result = fenceAgentExtractParams(params, nil)
	extractParamsMatch(result, []string{}, t)

	params["ip"] = "test_ip"
	params["username"] = "test_username"
	result = fenceAgentExtractParams(params, nil)
	extractParamsMatch(result,
		[]string{execPath, "--plug=" + params["plug"],
			"--ip=" + params["ip"],
			"--username=" + params["username"]},
		t)

	params["notls"] = "incorrect_bool_value"
	result = fenceAgentExtractParams(params, nil)
	extractParamsMatch(result, []string{}, t)

	params["notls"] = "on"
	result = fenceAgentExtractParams(params, nil)
	extractParamsMatch(result,
		[]string{execPath, "--plug=" + params["plug"],
			"--ip=" + params["ip"],
			"--username=" + params["username"], "--notls"},
		t)

	params["notls"] = "off"
	result = fenceAgentExtractParams(params, nil)
	extractParamsMatch(result,
		[]string{execPath, "--plug=" + params["plug"],
			"--ip=" + params["ip"],
			"--username=" + params["username"]},
		t)
}
