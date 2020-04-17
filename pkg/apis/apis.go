// Copyright 2019 The Kubernetes Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package apis contains Kubernetes API groups.
package apis

import (
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"

	logf "sigs.k8s.io/controller-runtime/pkg/log"

	dplapis "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis"
)

// AddToSchemes may be used to add all resources defined in the project to a Scheme
var AddToSchemes runtime.SchemeBuilder

// AddToScheme adds all Resources to the Scheme
func AddToScheme(s *runtime.Scheme) error {
	var err error
	// add mcm scheme, mcm scheme only on hub cluster
	if err = dplapis.AddToSchemes.AddToScheme(s); err != nil {
		logf.Log.Error(err, "unable add deployable APIs to scheme")
		return err
	}

	if err = clusterv1alpha1.AddToScheme(s); err != nil {
		logf.Log.Error(err, "unable add deployable APIs to scheme")
		return err
	}

	return AddToSchemes.AddToScheme(s)
}
