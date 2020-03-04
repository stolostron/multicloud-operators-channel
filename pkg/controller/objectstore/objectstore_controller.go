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

package objectstore

import (
	"context"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"
	gitsync "github.com/open-cluster-management/multicloud-operators-channel/pkg/synchronizer/githubsynchronizer"
	helmsync "github.com/open-cluster-management/multicloud-operators-channel/pkg/synchronizer/helmreposynchronizer"
	"github.com/open-cluster-management/multicloud-operators-channel/pkg/utils"
	dplv1 "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis/multicloudapps/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

const debugLevel = klog.Level(10)

// Add creates a new Deployable Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, recorder record.EventRecorder,
	channelDescriptor *utils.ChannelDescriptor, sync *helmsync.ChannelSynchronizer,
	gsync *gitsync.ChannelSynchronizer) error {
	return add(mgr, newReconciler(mgr, channelDescriptor))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, channelDescriptor *utils.ChannelDescriptor) reconcile.Reconciler {
	return &ReconcileDeployable{
		KubeClient:        mgr.GetClient(),
		ChannelDescriptor: channelDescriptor,
	}
}

type channelMapper struct {
	client.Client
}

func (mapper *channelMapper) Map(obj handler.MapObject) []reconcile.Request {
	dpllist := &dplv1.DeployableList{}

	err := mapper.List(context.TODO(), dpllist, &client.ListOptions{Namespace: obj.Meta.GetNamespace()})
	if err != nil {
		klog.Error("Failed to list all deployable: ", err)
		return nil
	}

	var requests []reconcile.Request
	for _, dpl := range dpllist.Items {
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Name: dpl.GetName(), Namespace: dpl.GetNamespace()}})
	}

	return requests
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("objectstore-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Deployable
	err = c.Watch(&source.Kind{Type: &dplv1.Deployable{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// watch for changes to channel too
	return c.Watch(&source.Kind{Type: &chv1.Channel{}}, &handler.EnqueueRequestsFromMapFunc{ToRequests: &channelMapper{mgr.GetClient()}})
}

var _ reconcile.Reconciler = &ReconcileDeployable{}

// ReconcileDeployable reconciles a Deployable object
type ReconcileDeployable struct {
	KubeClient        client.Client
	ChannelDescriptor *utils.ChannelDescriptor
}

// Reconcile reads that state of the cluster for a Deployable object and makes changes based on the state read
// and what is in the Deployable.Spec

// a Deployment as an example Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=multicloud-apps.io,resources=deployables,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=multicloud-apps.io,resources=deployables/status,verbs=get;update;patch
func (r *ReconcileDeployable) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Deployable instance
	instance := &dplv1.Deployable{}
	err := r.KubeClient.Get(context.TODO(), request.NamespacedName, instance)
	klog.Info("Reconciling:", request.NamespacedName, " with Get err:", err)

	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			_, err = r.deleteDeployableInObjectStore(request.NamespacedName)
			if err != nil {
				klog.Errorf("Failed to delete deployable %v  error %v", request.NamespacedName, err)
			}

			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		klog.V(debugLevel).Info("Reconciling - Errored.", request.NamespacedName, " with Get err:", err)

		return reconcile.Result{}, err
	}

	req, err := r.reconcileForChannel(instance)
	if err != nil {
		klog.Errorf("failed to reconcile deployable for channel %v, err %v", request.String(), err)
		return req, err
	}

	return reconcile.Result{}, err
}
