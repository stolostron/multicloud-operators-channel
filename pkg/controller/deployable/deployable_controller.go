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

package deployable

import (
	"context"
	"reflect"
	"strings"

	"k8s.io/klog"

	appv1alpha1 "github.com/IBM/multicloud-operators-channel/pkg/apis/app/v1alpha1"
	gitsync "github.com/IBM/multicloud-operators-channel/pkg/synchronizer/githubsynchronizer"
	helmsync "github.com/IBM/multicloud-operators-channel/pkg/synchronizer/helmreposynchronizer"
	"github.com/IBM/multicloud-operators-channel/pkg/utils"
	dplv1alpha1 "github.com/IBM/multicloud-operators-deployable/pkg/apis/app/v1alpha1"
	dplutils "github.com/IBM/multicloud-operators-deployable/pkg/utils"

	v1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
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

// Add creates a new Channel Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, recorder record.EventRecorder, channelDescriptor *utils.ChannelDescriptor, sync *helmsync.ChannelSynchronizer, gsync *gitsync.ChannelSynchronizer) error {
	return add(mgr, newReconciler(mgr, recorder))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, recorder record.EventRecorder) reconcile.Reconciler {
	return &ReconcileDeployable{Client: mgr.GetClient(), scheme: mgr.GetScheme(), Recorder: recorder}
}

type channelMapper struct {
	client.Client
}

func (mapper *channelMapper) Map(obj handler.MapObject) []reconcile.Request {
	if klog.V(10) {
		fnName := dplutils.GetFnName()
		klog.Infof("Entering: %v()", fnName)

		defer klog.Infof("Exiting: %v()", fnName)
	}

	dpllist := &dplv1alpha1.DeployableList{}

	err := mapper.List(context.TODO(), dpllist, &client.ListOptions{})
	if err != nil {
		klog.Error("Failed to list all deployable ")
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
	c, err := controller.New("deployable-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Deployable
	err = c.Watch(&source.Kind{Type: &dplv1alpha1.Deployable{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// watch for changes to channel too
	return c.Watch(&source.Kind{Type: &appv1alpha1.Channel{}}, &handler.EnqueueRequestsFromMapFunc{ToRequests: &channelMapper{mgr.GetClient()}})
}

var _ reconcile.Reconciler = &ReconcileDeployable{}

// ReconcileDeployable reconciles a Channel object
type ReconcileDeployable struct {
	client.Client
	scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

func (r *ReconcileDeployable) appendEvent(rootInstance *appv1alpha1.Channel, dplkey types.NamespacedName, derror error, reason, addtionalMsg string) error {
	phost := types.NamespacedName{Namespace: rootInstance.GetNamespace(), Name: rootInstance.GetName()}
	klog.V(10).Info("Promoted deployable: ", dplkey, ", channel: ", phost, ", message: ", addtionalMsg)

	eventType := ""
	evnetMsg := ""

	if derror != nil {
		eventType = v1.EventTypeWarning
		evnetMsg = addtionalMsg + ", Status: Failed, " + "Channel: " + phost.String()
	} else {
		eventType = v1.EventTypeNormal
		evnetMsg = addtionalMsg + ", Status: Success, " + "Channel: " + phost.String()
	}

	r.Recorder.Event(rootInstance, eventType, reason, evnetMsg)

	return nil
}

// Reconcile reads that state of the cluster for a Channel object and makes changes based on the state read
// and what is in the Channel.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  The scaffolding writes
// a Deployment as an example
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=app.ibm.com,resources=channels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=app.ibm.com,resources=channels/status,verbs=get;update;patch
func (r *ReconcileDeployable) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Channel instance
	if klog.V(10) {
		fnName := dplutils.GetFnName()
		klog.Infof("Entering: %v()", fnName)

		defer klog.Infof("Exiting: %v()", fnName)
	}

	klog.Info("Reconciling:", request.NamespacedName)

	instance := &dplv1alpha1.Deployable{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)

	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			klog.V(10).Info(err)
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	channelmap, err := utils.GenerateChannelMap(r.Client)
	if err != nil {
		if !errors.IsNotFound(err) {
			klog.Error("Failed to get all deployables")
			return reconcile.Result{}, nil
		}

		channelmap = make(map[string]*appv1alpha1.Channel)
	}

	channelnsMap := make(map[string]string)
	for _, ch := range channelmap {
		channelnsMap[ch.Namespace] = ch.Name
	}

	parent, dplmap, err := utils.FindDeployableForChannelsInMap(r.Client, instance, channelnsMap)
	if err != nil && !errors.IsNotFound(err) {
		klog.Error("Failed to get all deployables")
		return reconcile.Result{}, nil
	}

	klog.V(10).Infof("Dpl Map, before deletion: %#v", dplmap)

	if len(instance.GetFinalizers()) == 0 {
		annotations := instance.Annotations
		if channelnsMap[instance.Namespace] != "" && annotations != nil && annotations[appv1alpha1.KeyChannelSource] != "" && parent == nil {
			klog.V(10).Infof("Delete instance: The parent of the instance not found: %#v, %#v", annotations[appv1alpha1.KeyChannelSource], instance)
			return reconcile.Result{}, r.Client.Delete(context.TODO(), instance)
		}

		for _, chname := range instance.Spec.Channels {
			ch, ok := channelmap[chname]
			if !ok {
				klog.Error("Failed to find channel name:", chname, " for deployable ", instance.Namespace, "/", instance.Name, " err: ", err)
				continue
			}

			err = r.propagateDeployableToChannel(instance, dplmap, ch)
			if err != nil {
				klog.Error("Failed to validate deplyable for ", instance)
			}

			delete(channelmap, chname)
		}

		for _, ch := range channelmap {
			err = r.propagateDeployableToChannel(instance, dplmap, ch)
			if err != nil {
				klog.Errorf("Failed to propagate %v To Channel due to %v ", instance, err)
			}
		}
	}

	//If the dpl changes its channel, delete all other children of the dpl who were propagated to other channels before.
	for chstr, dpl := range dplmap {
		klog.Info("deleting deployable ", dpl.Namespace, "/", dpl.Name, " from channel ", chstr)
		err = r.Client.Delete(context.TODO(), dpl)
		if err != nil {
			klog.Errorf("Failed to delete %v due to %v ", dpl.Name, err)
		}

		//record events
		dplkey := types.NamespacedName{Name: dpl.GetName(), Namespace: dpl.GetNamespace()}
		addtionalMsg := "Depolyable " + dplkey.String() + " removed from the channel"

		dplchannel := &appv1alpha1.Channel{}

		parsedstr := strings.Split(chstr, "/")
		if len(parsedstr) != 2 {
			klog.Info("invalid channel namespacedName: ", chstr)

			continue
		}

		chkey := types.NamespacedName{Name: parsedstr[1], Namespace: parsedstr[0]}

		error1 := r.Get(context.TODO(), chkey, dplchannel)
		if error1 != nil {
			klog.V(5).Infof("Channel %#v not found, unable to record events to it. ", chkey)
		} else {
			err = r.appendEvent(dplchannel, dplkey, err, "Delete", addtionalMsg)
			if err != nil {
				klog.Errorf("Failed to record event %v due to %v ", dplkey, err)
			}
		}
	}

	return reconcile.Result{}, err
}

func (r *ReconcileDeployable) propagateDeployableToChannel(deployable *dplv1alpha1.Deployable, dplmap map[string]*dplv1alpha1.Deployable, channel *appv1alpha1.Channel) error {
	if klog.V(10) {
		fnName := dplutils.GetFnName()
		klog.Infof("Entering: %v()", fnName)

		defer klog.Infof("Exiting: %v()", fnName)
	}

	chkey := types.NamespacedName{Name: channel.Name, Namespace: channel.Namespace}.String()

	if deployable.Namespace == channel.Namespace {
		klog.V(10).Infof("The deployable: %#v exists in channel: %#v.", deployable.GetName(), channel.GetName())

		delete(dplmap, chkey)

		return nil
	}

	// ValidateDeployableToChannel check if a deployable can be promoted to channel

	// promote path:
	// a, dpl has channel spec
	// a.0  .0 the current channe match the spec
	// a.0, the gate on channel is empty, then promote
	// //a.1  the gate on channel is not empty, then
	// ////a.1.0, if dpl annotation is empty, fail
	// ////a.1.1, if dpl annotation has a match the gate annotation, then promote

	// b, the dpl doesn't have channel spec
	// b.0 if channel doesn't have a gate, then fail
	// b.1 if channel's namespace source is the same as dpl
	// // b.1.1 if gate and dpl annotation has a match then promote
	// // b.1.1 dpl doesn't have annotation, then fail
	if !utils.ValidateDeployableToChannel(deployable, channel) {
		klog.V(10).Infof("The deployable %#v can't be promoted to channel %#v.", deployable.GetName(), channel.GetName())
		return nil
	}

	chdpl, err := utils.GenerateDeployableForChannel(deployable, types.NamespacedName{Name: channel.Name, Namespace: channel.Namespace})
	if err != nil {
		return err
	}

	exdpl, ok := dplmap[chkey]

	if !ok {
		klog.Info("Creating deployable in channel", *chdpl)
		err = r.Client.Create(context.TODO(), chdpl)

		//record events
		dplkey := types.NamespacedName{Name: chdpl.GetName(), Namespace: chdpl.GetNamespace()}
		addtionalMsg := "Depolyable " + dplkey.String() + " created in the channel"

		err = r.appendEvent(channel, dplkey, err, "Deploy", addtionalMsg)
		if err != nil {
			klog.Errorf("Failed to record event %v due to %v ", dplkey, err)
		}

		return err
	}

	if reflect.DeepEqual(exdpl.GetAnnotations(), chdpl.GetAnnotations()) && reflect.DeepEqual(exdpl.GetLabels(), chdpl.GetLabels()) && reflect.DeepEqual(exdpl.Spec, chdpl.Spec) {
		klog.Info("No changes to existing deployable in channel ", *exdpl)
	} else {
		exdpl.SetLabels(chdpl.GetLabels())
		exdpl.SetAnnotations(chdpl.GetAnnotations())
		chdpl.Spec.DeepCopyInto(&(exdpl.Spec))
		err = r.Client.Update(context.TODO(), exdpl)
		klog.Info("Updating existing deployable in channel to ", *exdpl)

		//record events
		dplkey := types.NamespacedName{Name: exdpl.GetName(), Namespace: exdpl.GetNamespace()}
		addtionalMsg := "Depolyable " + dplkey.String() + " updated in the channel"

		err = r.appendEvent(channel, dplkey, err, "Deploy", addtionalMsg)
		if err != nil {
			klog.Errorf("Failed to record event %v due to %v ", dplkey, err)
		}
	}

	delete(dplmap, chkey)

	return err
}
