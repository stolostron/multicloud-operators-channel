// Copyright 2021 The Kubernetes Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package webhook

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// TestCreateOrRecreate_RaceOnAlreadyExists is a regression test for a flake seen in CI:
//
//	failed to set up the validation webhook config: failed to set up service for webhook:
//	services "channels-apps-open-cluster-management-webhook-svc" already exists
//
// This happens when a "does it exist" check is served from a client-side informer cache
// (e.g. mgr.GetClient()) that hasn't yet observed a concurrent/previous create of the same
// object, so the code proceeds to Create() and the API server rejects it with AlreadyExists.
// createOrRecreate should recover by deleting the stale object and retrying the create.
func TestCreateOrRecreate_RaceOnAlreadyExists(t *testing.T) {
	existing := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "channels-apps-open-cluster-management-webhook-svc", Namespace: "default"},
	}

	createCalls := 0

	fakeClient := fake.NewClientBuilder().
		WithObjects(existing).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				createCalls++
				if createCalls == 1 {
					// Simulate: the object already exists on the server (race),
					// even though the caller's stale cache read said otherwise.
					return apierrors.NewAlreadyExists(schema.GroupResource{Resource: "services"}, obj.GetName())
				}

				return c.Create(ctx, obj, opts...)
			},
		}).
		Build()

	newSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "channels-apps-open-cluster-management-webhook-svc", Namespace: "default"},
	}

	if err := createOrRecreate(context.TODO(), fakeClient, newSvc, logr.Discard(), "test service"); err != nil {
		t.Fatalf("expected createOrRecreate to recover from AlreadyExists, got error: %v", err)
	}

	if createCalls != 2 {
		t.Fatalf("expected exactly 2 create attempts (initial + retry after delete), got %d", createCalls)
	}

	got := &corev1.Service{}
	if err := fakeClient.Get(context.TODO(), client.ObjectKeyFromObject(newSvc), got); err != nil {
		t.Fatalf("expected service to exist after createOrRecreate, got error: %v", err)
	}
}

// TestCreateOrRecreate_PropagatesOtherErrors ensures non-AlreadyExists errors are
// returned as-is without attempting a delete/retry.
func TestCreateOrRecreate_PropagatesOtherErrors(t *testing.T) {
	fakeClient := fake.NewClientBuilder().
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				return apierrors.NewForbidden(schema.GroupResource{Resource: "services"}, obj.GetName(), nil)
			},
		}).
		Build()

	newSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "some-svc", Namespace: "default"},
	}

	err := createOrRecreate(context.TODO(), fakeClient, newSvc, logr.Discard(), "test service")
	if err == nil {
		t.Fatalf("expected error to propagate, got nil")
	}

	if !apierrors.IsForbidden(err) {
		t.Fatalf("expected Forbidden error to propagate unchanged, got: %v", err)
	}
}
