// Licensed Materials - Property of IBM
// (c) Copyright IBM Corporation 2016, 2019. All Rights Reserved.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP  Schedule Contract with IBM Corp.

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

var (
	// KeyChannelSource is namespacedname tells the source of the deployable
	KeyChannelSource = SchemeGroupVersion.Group + "/hosting-deployable"
	// KeyChannel is namespacedname tells the source of the channel
	KeyChannel = SchemeGroupVersion.Group + "/channel"
	// ServingChannel tells the channel that the secrect or configMap refers to
	ServingChannel = SchemeGroupVersion.Group + "/serving-channel"
)

// ChannelType defines types of channel
type ChannelType string

const (
	// ChannelTypeNamespace defines type name of namespace channel
	ChannelTypeNamespace = "namespace"
	// ChannelTypeHelmRepo defines type name of helm repository channel
	ChannelTypeHelmRepo = "helmrepo"
	// ChannelTypeObjectBucket defines type name of bucket in object store
	ChannelTypeObjectBucket = "objectbucket"
	// ChannelTypeGitHub defines type name of GitHub repository
	ChannelTypeGitHub = "github"
)

// ChannelGate defines criteria for promote to channel
type ChannelGate struct {
	Name          string                `json:"name,omitempty"`
	LabelSelector *metav1.LabelSelector `json:"labelSelector,omitempty"`
	Annotations   map[string]string     `json:"annotations,omitempty"`
}

// ChannelSpec defines the desired state of Channel
type ChannelSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// +kubebuilder:validation:Enum=Namespace,HelmRepo,ObjectBucket,GitHub,namespace,helmrepo,objectbucket,github
	Type         ChannelType             `json:"type"`
	PathName     string                  `json:"pathName"`
	SecretRef    *corev1.ObjectReference `json:"secretRef,omitempty"`
	ConfigMapRef *corev1.ObjectReference `json:"configMapRef,omitempty"`
	Gates        *ChannelGate            `json:"gates,omitempty"`
	// +listType=set
	SourceNamespaces []string `json:"sourceNamespaces,omitempty"`
}

// ChannelStatus defines the observed state of Channel
type ChannelStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Channel is the Schema for the channels API
// +k8s:openapi-gen=true
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type",description="type of the channel"
// +kubebuilder:printcolumn:name="PathName",type="string",JSONPath=".spec.pathname",description="pathname of the channel"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type Channel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ChannelSpec   `json:"spec,omitempty"`
	Status ChannelStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ChannelList contains a list of Channel
type ChannelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// +listType=set
	Items []Channel `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Channel{}, &ChannelList{})
}
