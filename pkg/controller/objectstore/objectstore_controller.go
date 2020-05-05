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
	"fmt"

	"github.com/go-logr/logr"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"
	helmsync "github.com/open-cluster-management/multicloud-operators-channel/pkg/synchronizer/helmreposynchronizer"
	"github.com/open-cluster-management/multicloud-operators-channel/pkg/utils"
	dplv1 "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis/apps/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
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

const (
	controllerName  = "objectbucket"
	controllerSetup = "objectbucket-setup"
)

// Add creates a new Deployable Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, recorder record.EventRecorder, logger logr.Logger,
	channelDescriptor *utils.ChannelDescriptor, sync *helmsync.ChannelSynchronizer) error {
	return add(mgr, newReconciler(mgr, channelDescriptor, logger.WithName(controllerName)), logger.WithName(controllerSetup))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, channelDescriptor *utils.ChannelDescriptor, logger logr.Logger) reconcile.Reconciler {
	return &ReconcileDeployable{
		KubeClient:        mgr.GetClient(),
		ChannelDescriptor: channelDescriptor,
		Log:               logger,
	}
}

type channelMapper struct {
	client.Client
	logger logr.Logger
}

func (mapper *channelMapper) Map(obj handler.MapObject) []reconcile.Request {
	dpllist := &dplv1.DeployableList{}

	err := mapper.List(context.TODO(), dpllist, client.InNamespace(obj.Meta.GetNamespace()))
	if err != nil {
		mapper.logger.Error(err, "failed to list all deployable")
		return nil
	}

	var requests []reconcile.Request
	for _, dpl := range dpllist.Items {
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Name: dpl.GetName(), Namespace: dpl.GetNamespace()}})
	}

	return requests
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler, logger logr.Logger) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Deployable
	err = c.Watch(&source.Kind{Type: &dplv1.Deployable{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// watch for changes to channel too
	return c.Watch(&source.Kind{
		Type: &chv1.Channel{}},
		&handler.EnqueueRequestsFromMapFunc{ToRequests: &channelMapper{Client: mgr.GetClient(), logger: logger}})
}

var _ reconcile.Reconciler = &ReconcileDeployable{}

// ReconcileDeployable reconciles a Deployable object
type ReconcileDeployable struct {
	KubeClient        client.Client
	ChannelDescriptor *utils.ChannelDescriptor
	Log               logr.Logger
}

// Reconcile reads that state of the cluster for a Deployable object, make sure the deployables at cluster has accurate record on objectstore store
/// if deployable is updated or created, then make sure the object bucket has a record of it as well. If
// if deployable is deleted, then it also delete the record of the deployable from object bucket.
// Note, within this flow, we only take above actions for deployables who's annotated with dplv1.AnnotationHosting

// a Deployment as an example Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=apps.open-cluster-management.io,resources=deployables,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.open-cluster-management.io,resources=deployables/status,verbs=get;update;patch
func (r *ReconcileDeployable) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Deployable instance
	log := r.Log.WithValues("obj-reconcile", request.NamespacedName)
	log.Info(fmt.Sprintf("Starting %v reconcile loop for %v", controllerName, request.NamespacedName))

	defer log.Info(fmt.Sprintf("Finish %v reconcile loop for %v", controllerName, request.NamespacedName))

	instance := &dplv1.Deployable{}
	err := r.KubeClient.Get(context.TODO(), request.NamespacedName, instance)

	if err != nil {
		if errors.IsNotFound(err) {
			if err := r.deleteDeployableInObjectBucket(request.NamespacedName, log); err != nil {
				log.Error(err, fmt.Sprintf("failed to delete deployable %v", request.NamespacedName))
				return reconcile.Result{}, err
			}

			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Reconciling - Errored.")

		return reconcile.Result{}, err
	}

	if err := r.createOrUpdateDeployableInObjectBucket(instance, log); err != nil {
		log.Error(err, fmt.Sprintf("failed to reconcile deployable for channel %v", request.String()))
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}
