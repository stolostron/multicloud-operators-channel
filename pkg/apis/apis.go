// Licensed Materials - Property of IBM
// (c) Copyright IBM Corporation 2016, 2019. All Rights Reserved.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP  Schedule Contract with IBM Corp.

// Generate deepcopy for apis
//go:generate go run ../../vendor/k8s.io/code-generator/cmd/deepcopy-gen/main.go -O zz_generated.deepcopy -i ./... -h ../../hack/boilerplate.go.txt

// Package apis contains Kubernetes API groups.
package apis

import (
	dplv1alpha1 "github.com/IBM/multicloud-operators-deployable/pkg/apis"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	"k8s.io/klog"
)

// AddToSchemes may be used to add all resources defined in the project to a Scheme
var AddToSchemes runtime.SchemeBuilder

// AddToScheme adds all Resources to the Scheme
func AddToScheme(s *runtime.Scheme) error {

	var err error
	// add mcm scheme, mcm scheme only on hub cluster
	if err = dplv1alpha1.AddToScheme(s); err != nil {
		klog.Error("unable add deployable APIs to scheme", err)
		return err
	}
	if err = clusterv1alpha1.AddToScheme(s); err != nil {
		klog.Error("unable add deployable APIs to scheme", err)
		return err
	}

	return AddToSchemes.AddToScheme(s)
}
