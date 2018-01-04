/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	"encoding/json"

	core_v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	NodeFenceResourcePlural = "nodefences"
)

// NodeFenceStatus is the status of the NodeFence
type NodeFenceStatus struct {
	// The time the fencing was successfully created
	// +optional
	CreationTimestamp metav1.Time `json:"creationTimestamp"`

	// Represent the latest available observations
	Conditions []NodeFenceCondition `json:"conditions"`
}

// NodeFenceConditionType is the type of NodeFence conditions
type NodeFenceConditionType string

// These are valid conditions of node fencing
const (
	// NodeFenceConditionRunning means the node fencing is being executed
	NodeFenceConditionRunning NodeFenceConditionType = "Running"
	// NodeFenceConditionDone is added when the node is successfully executed
	NodeFenceConditionDone NodeFenceConditionType = "Done"
	// NodeFenceConditionError means an error occurred during node fencing.
	NodeFenceConditionError NodeFenceConditionType = "Error"
	// NodeFenceConditionNew new created fence object.
	NodeFenceConditionNew NodeFenceConditionType = "New"
)

// NodeFenceCondition describes the state of node fencing
type NodeFenceCondition struct {
	// Type of replication controller condition.
	Type NodeFenceConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status core_v1.ConditionStatus `json:"status"`
	// The last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime"`
	// The reason for the condition's last transition.
	// +optional
	Reason string `json:"reason"`
	// A human readable message indicating details about the transition.
	// +optional
	Message string `json:"message"`
}

// NodeFenceStepType type for nodefence steps
type NodeFenceStepType string

const (
	// NodeFenceStepIsolation means the fence process in isolation phase
	NodeFenceStepIsolation NodeFenceStepType = "Isolation"
	// NodeFenceStepPowerManagement means the fence process in pm phase
	NodeFenceStepPowerManagement NodeFenceStepType = "Power-Management"
	// NodeFenceStepRecovery means the fence process in recovery phase
	NodeFenceStepRecovery NodeFenceStepType = "Recovery"
)

// +genclient=true

// NodeFence is the node fencing object accessible to the fencing controller and executor
type NodeFence struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ObjectMeta `json:"metadata"`

	// Node represents the node to be fenced.
	NodeName string `json:"node"`

	// Step represent the current step in the fence operation
	Step NodeFenceStepType `json:"step"`

	// boolean represent if controller manage node's resource during fence
	CleanResources bool `json:"clean_resources"`

	// PV presents the persistent volume attached/mounted on the node
	// +optional
	//PV core_v1.PersistentVolume `json:"pv"`

	// Status represents the latest observer state of the node fencing
	//Status NodeFenceStatus `json:"status"`
	Status NodeFenceConditionType `json:"status"`

	// If running hostname is set with executor hostname
	Hostname string `json:"hostname"`
}

// NodeFenceList is a list of NodeFence objects
type NodeFenceList struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ListMeta `json:"metadata"`
	Items           []NodeFence     `json:"items"`
}

// NodeFenceConfig holds configmap values
type NodeFenceConfig struct {
	NodeName        string   `json:"node"`
	PowerManagement []string `json:"power-management"`
	Isolation       []string `json:"isolation"`
	Recovery        []string `json:"recovery"`
}

// GetObjectKind is required to satisfy Object interface
func (n *NodeFence) GetObjectKind() schema.ObjectKind {
	return &n.TypeMeta
}

// GetObjectMeta is required to satisfy ObjectMetaAccessor interface
func (n *NodeFence) GetObjectMeta() metav1.Object {
	return &n.Metadata
}

// GetObjectKind is required to satisfy Object interface
func (nd *NodeFenceList) GetObjectKind() schema.ObjectKind {
	return &nd.TypeMeta
}

// GetListMeta is required to satisfy ListMetaAccessor interface
func (nd *NodeFenceList) GetListMeta() metav1.List {
	return &nd.Metadata
}

// NodeFenceListCopy is a NodeFenceList type
type NodeFenceListCopy NodeFenceList

// NodeFenceCopy is a NodeFence type
type NodeFenceCopy NodeFence

// UnmarshalJSON unmarshalls json data
func (v *NodeFence) UnmarshalJSON(data []byte) error {
	tmp := NodeFenceCopy{}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	tmp2 := NodeFence(tmp)
	*v = tmp2
	return nil
}

// UnmarshalJSON unmarshals json data
func (vd *NodeFenceList) UnmarshalJSON(data []byte) error {
	tmp := NodeFenceListCopy{}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	tmp2 := NodeFenceList(tmp)
	*vd = tmp2
	return nil
}

type ContentType uint8

const (
	Boolean ContentType = iota
	String
	Select
)

// Parameter describes parameter object
type Parameter struct {
	// Parameter Name
	Name string

	// If false the parameter can be specified multiple times
	Unique bool

	// If true the parameter is required
	Required bool

	// Parameter description
	Desc string

	// Parameter Type
	ContentType ContentType

	// Default value. If nil no default values is defined.
	Default interface{}

	// If true the parameter's accepted values are provided by Options.
	HasOptions bool

	// Accepted parameter's values.
	Options []interface{}
}

// Action index type for ActionMap
type Action uint8

// Available actions
const (
	// No Action.
	None Action = iota
	// PowerOn Machine, disable port access etc...
	On
	// PowerOff Machine, disable port access etc...
	Off
	// Reboot Machine.
	Reboot
	// Get Machime, port etc... status
	Status
	// List available "ports". A port can be a virtual machine, a power
	// port, a switch port etc... managed by this fence devices
	List
	// Check the health of the fence device itself
	Monitor
)

// Action internal value to name translation. Used for logging and errors.
var ActionMap = map[Action]string{
	None:    "none",
	On:      "on",
	Off:     "off",
	Reboot:  "reboot",
	Status:  "status",
	List:    "list",
	Monitor: "monitor",
}
