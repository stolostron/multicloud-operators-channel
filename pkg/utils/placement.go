// Copyright 2021 The Kubernetes Authors.
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

package utils

import (
	"context"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
	spokeClusterV1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IsReadyClusterRegistry check if Cluster API service is ready or not.
func IsReadyClusterRegistry(clReader client.Reader) bool {
	cllist := &spokeClusterV1.ManagedClusterList{}

	listopts := &client.ListOptions{}

	err := clReader.List(context.TODO(), cllist, listopts)

	if err == nil {
		klog.Error("Cluster API service ready")
		return true
	}

	klog.Error("Cluster API service NOT ready: ", err)

	return false
}

// DetectClusterRegistry - Detect the cluster API service every 10 seconds. the controller will be exited when it is ready
// The controller will be auto restarted by the multicluster-operators-application deployment CR later.
//
//nolint:unparam
func DetectClusterRegistry(ctx context.Context, clReader client.Reader) {
	if !IsReadyClusterRegistry(clReader) {
		go wait.UntilWithContext(ctx, func(ctx context.Context) {
			if IsReadyClusterRegistry(clReader) {
				os.Exit(1)
			}
		}, time.Duration(10)*time.Second)
	}
}
