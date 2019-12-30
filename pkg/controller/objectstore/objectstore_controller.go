// Licensed Materials - Property of IBM
// (c) Copyright IBM Corporation 2016, 2019. All Rights Reserved.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP  Schedule Contract with IBM Corp.

package objectstore

import (
	"context"

	chnv1alpha1 "github.com/IBM/multicloud-operators-channel/pkg/apis/app/v1alpha1"
	gitsync "github.com/IBM/multicloud-operators-channel/pkg/synchronizer/githubsynchronizer"
	helmsync "github.com/IBM/multicloud-operators-channel/pkg/synchronizer/helmreposynchronizer"
	"github.com/IBM/multicloud-operators-channel/pkg/utils"
	appv1alpha1 "github.com/IBM/multicloud-operators-deployable/pkg/apis/app/v1alpha1"

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

// Add creates a new Deployable Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, recorder record.EventRecorder, channelDescriptor *utils.ChannelDescriptor, sync *helmsync.ChannelSynchronizer, gsync *gitsync.ChannelSynchronizer) error {

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

	dpllist := &appv1alpha1.DeployableList{}
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
	err = c.Watch(&source.Kind{Type: &appv1alpha1.Deployable{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// watch for changes to channel too
	err = c.Watch(&source.Kind{Type: &chnv1alpha1.Channel{}}, &handler.EnqueueRequestsFromMapFunc{ToRequests: &channelMapper{mgr.GetClient()}})

	return err
}

var _ reconcile.Reconciler = &ReconcileDeployable{}

// ReconcileDeployable reconciles a Deployable object
type ReconcileDeployable struct {
	KubeClient        client.Client
	ChannelDescriptor *utils.ChannelDescriptor
}

// Reconcile reads that state of the cluster for a Deployable object and makes changes based on the state read
// and what is in the Deployable.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  The scaffolding writes
// a Deployment as an example
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=app.ibm.com,resources=deployables,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=app.ibm.com,resources=deployables/status,verbs=get;update;patch
func (r *ReconcileDeployable) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Deployable instance
	instance := &appv1alpha1.Deployable{}
	err := r.KubeClient.Get(context.TODO(), request.NamespacedName, instance)
	klog.Info("Reconciling:", request.NamespacedName, " with Get err:", err)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.

			r.deleteDeployableInObjectStore(request.NamespacedName)

			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		klog.V(10).Info("Reconciling - Errored.", request.NamespacedName, " with Get err:", err)
		return reconcile.Result{}, err
	}

	_, err = r.reconcileForChannel(instance)
	if err != nil {
		klog.Error("Failed to reconcile deployable for channel")
	}

	return reconcile.Result{}, err
}
