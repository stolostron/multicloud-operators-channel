// Copyright 2020 The Kubernetes Authors.
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
	"testing"

	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	spokeClusterV1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

var (
	oldClusterCond1 = metav1.Condition{
		Type:   "ManagedClusterConditionAvailable",
		Status: "true",
	}

	oldClusterCond2 = metav1.Condition{
		Type:   "HubAcceptedManagedCluster",
		Status: "true",
	}

	oldCluster = &spokeClusterV1.ManagedCluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ManagedCluster",
			APIVersion: "cluster.open-cluster-management.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster1",
			Labels: map[string]string{
				"name": "cluster1",
				"key1": "c1v1",
				"key2": "c1v2",
			},
		},
		Status: spokeClusterV1.ManagedClusterStatus{
			Conditions: []metav1.Condition{oldClusterCond1, oldClusterCond2},
		},
	}

	newClusterCond = metav1.Condition{
		Type:   "ManagedClusterConditionAvailable",
		Status: "true",
	}

	newCluster = &spokeClusterV1.ManagedCluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ManagedCluster",
			APIVersion: "cluster.open-cluster-management.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster1",
			Labels: map[string]string{
				"name": "cluster1",
				"key1": "c1v1",
				"key2": "c1v2",
			},
		},
		Status: spokeClusterV1.ManagedClusterStatus{
			Conditions: []metav1.Condition{newClusterCond},
		},
	}
)

func TestPredicate(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Test ClusterPredicateFunc
	instance := ClusterPredicateFunc

	updateEvt := event.UpdateEvent{
		ObjectOld: oldCluster,
		ObjectNew: newCluster,
	}
	ret := instance.Update(updateEvt)
	g.Expect(ret).To(gomega.Equal(true))
}
