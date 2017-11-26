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
	// NodeFencingResourcePlural is "nodefencings"
	NodeFencingResourcePlural = "nodefencings"
)

// NodeFencingStatus is the status of the NodeFencing
type NodeFencingStatus struct {
	// The time the fencing was successfully created
	// +optional
	CreationTimestamp metav1.Time `json:"creationTimestamp" protobuf:"bytes,1,opt,name=creationTimestamp"`

	// Represent the latest available observations
	Conditions []NodeFencingCondition `json:"conditions" protobuf:"bytes,2,rep,name=conditions"`
}

// NodeFencingConditionType is the type of NodeFencing conditions
type NodeFencingConditionType string

// These are valid conditions of node fencing
const (
	// NodeFencingConditionPending means the node fencing is being executed
	NodeFencingConditionPending NodeFencingConditionType = "Pending"
	// NodeFencingConditionReady is added when the node is successfully executed
	NodeFencingConditionReady NodeFencingConditionType = "Ready"
	// NodeFencingConditionError means an error occurred during node fencing.
	NodeFencingConditionError NodeFencingConditionType = "Error"
)

// NodeFencingCondition describes the state of node fencing
type NodeFencingCondition struct {
	// Type of replication controller condition.
	Type NodeFencingConditionType `json:"type" protobuf:"bytes,1,opt,name=type,casttype=NodeFencingConditionType"`
	// Status of the condition, one of True, False, Unknown.
	Status core_v1.ConditionStatus `json:"status" protobuf:"bytes,2,opt,name=status,casttype=ConditionStatus"`
	// The last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime" protobuf:"bytes,3,opt,name=lastTransitionTime"`
	// The reason for the condition's last transition.
	// +optional
	Reason string `json:"reason" protobuf:"bytes,4,opt,name=reason"`
	// A human readable message indicating details about the transition.
	// +optional
	Message string `json:"message" protobuf:"bytes,5,opt,name=message"`
}

// +genclient=true

// NodeFencing is the node fencing object accessible to the fencing controller and executor
type NodeFencing struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ObjectMeta `json:"metadata"`

	// Node represents the node to be fenced.
	// +optional
	Node core_v1.Node `json:"node" protobuf:"bytes,2,opt,name=node"`

	// PV presents the persistent volume attached/mounted on the node
	// +optional
	PV core_v1.PersistentVolume `json:"pv" protobuf:"bytes,3,opt,name=pv"`

	// Status represents the latest observer state of the node fencing
	// +optional
	Status NodeFencingStatus `json:"status" protobuf:"bytes,4,opt,name=status"`
}

// NodeFencingList is a list of NodeFencing objects
type NodeFencingList struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ListMeta `json:"metadata"`
	Items           []NodeFencing   `json:"items"`
}

// NodeFenceConfig holds configmap values
type NodeFenceConfig struct {
	NodeName        string   `json:"name" protobuf:"bytes,5,opt,name=node_name"`
	PowerManagement []string `json:"items"`
	Isolation       []string `json:"items"`
	Recovery        []string `json:"items"`
}

// GetObjectKind is required to satisfy Object interface
func (n *NodeFencing) GetObjectKind() schema.ObjectKind {
	return &n.TypeMeta
}

// GetObjectMeta is required to satisfy ObjectMetaAccessor interface
func (n *NodeFencing) GetObjectMeta() metav1.Object {
	return &n.Metadata
}

// GetObjectKind is required to satisfy Object interface
func (nd *NodeFencingList) GetObjectKind() schema.ObjectKind {
	return &nd.TypeMeta
}

// GetListMeta is required to satisfy ListMetaAccessor interface
func (nd *NodeFencingList) GetListMeta() metav1.List {
	return &nd.Metadata
}

// NodeFencingListCopy is a NodeFencingList type
type NodeFencingListCopy NodeFencingList

// NodeFencingCopy is a NodeFencing type
type NodeFencingCopy NodeFencing

// UnmarshalJSON unmarshalls json data
func (v *NodeFencing) UnmarshalJSON(data []byte) error {
	tmp := NodeFencingCopy{}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	tmp2 := NodeFencing(tmp)
	*v = tmp2
	return nil
}

// UnmarshalJSON unmarshals json data
func (vd *NodeFencingList) UnmarshalJSON(data []byte) error {
	tmp := NodeFencingListCopy{}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	tmp2 := NodeFencingList(tmp)
	*vd = tmp2
	return nil
}
