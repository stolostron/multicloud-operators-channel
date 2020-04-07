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
	"fmt"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"
	gitsync "github.com/open-cluster-management/multicloud-operators-channel/pkg/synchronizer/githubsynchronizer"
	helmsync "github.com/open-cluster-management/multicloud-operators-channel/pkg/synchronizer/helmreposynchronizer"
	"github.com/open-cluster-management/multicloud-operators-channel/pkg/utils"
	dplv1 "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis/apps/v1"
	"github.com/prometheus/common/log"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog"

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

const (
	reconcileName = "deployable-reconcile"
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new Channel Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, recorder record.EventRecorder, logger logr.Logger,
	channelDescriptor *utils.ChannelDescriptor, sync *helmsync.ChannelSynchronizer,
	gsync *gitsync.ChannelSynchronizer) error {
	return add(mgr, newReconciler(mgr, recorder, logger.WithName(reconcileName)), logger.WithName("deployable-setup"))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, recorder record.EventRecorder, logger logr.Logger) reconcile.Reconciler {
	return &ReconcileDeployable{Client: mgr.GetClient(), scheme: mgr.GetScheme(), Recorder: recorder, Log: logger}
}

type channelMapper struct {
	client.Client
	log logr.Logger
}

func (mapper *channelMapper) Map(obj handler.MapObject) []reconcile.Request {
	dpllist := &dplv1.DeployableList{}

	err := mapper.List(context.TODO(), dpllist, &client.ListOptions{})
	if err != nil {
		mapper.log.Error(err, "failed to list all deployable ")
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
	c, err := controller.New("deployable-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Deployable
	err = c.Watch(&source.Kind{Type: &dplv1.Deployable{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// watch for changes to channel too
	return c.Watch(&source.Kind{Type: &chv1.Channel{}}, &handler.EnqueueRequestsFromMapFunc{ToRequests: &channelMapper{Client: mgr.GetClient(), log: logger}})
}

var _ reconcile.Reconciler = &ReconcileDeployable{}

// ReconcileDeployable reconciles a Channel object
type ReconcileDeployable struct {
	client.Client
	scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Log      logr.Logger
}

func (r *ReconcileDeployable) appendEvent(rootInstance *chv1.Channel, dplkey types.NamespacedName, derror error, reason, addtionalMsg string) error {
	phost := types.NamespacedName{Namespace: rootInstance.GetNamespace(), Name: rootInstance.GetName()}
	r.Log.Info(fmt.Sprintf("Promoted deployable: %v channel: %v message: %v ", dplkey, phost, addtionalMsg))

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
// a Deployment as an example
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.open-cluster-management.io,resources=channels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.open-cluster-management.io,resources=channels/status,verbs=get;update;patch
func (r *ReconcileDeployable) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Channel instance
	r.Log = r.Log.WithValues("dpl-reconcile", request.NamespacedName)

	r.Log.Info(fmt.Sprintf("Starting dpl reconcile loop for %v", request.NamespacedName))
	defer r.Log.Info(fmt.Sprintf("Finish reconcile loop for %v", request.NamespacedName))

	instance := &dplv1.Deployable{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)

	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			r.Log.Info("exit, deployables will be GC'ed by k8s")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	channelmap, err := utils.GenerateChannelMap(r.Client)
	if err != nil {
		if !errors.IsNotFound(err) {
			r.Log.Error(err, "failed to get all deployables")
			return reconcile.Result{}, nil
		}

		channelmap = make(map[string]*chv1.Channel)
	}

	channelNsMap := make(map[string]string)
	for _, ch := range channelmap {
		channelNsMap[ch.Namespace] = ch.Name
	}

	parent, dplmap, err := utils.FindDeployableForChannelsInMap(r.Client, instance, channelNsMap)
	if err != nil && !errors.IsNotFound(err) {
		log.Error(err, "failed to get all deployables")
		return reconcile.Result{}, nil
	}

	r.Log.Info(fmt.Sprintf("Dpl Map, before deletion: %#v", dplmap))

	dplmap, err = r.updateDeployableRelationWithChannel(instance, dplmap, parent, channelNsMap, channelmap)
	if err != nil {
		return reconcile.Result{}, err
	}

	r.handleOrphanDeployable(dplmap)

	return reconcile.Result{}, err
}

func (r *ReconcileDeployable) handleOrphanDeployable(dplmap map[string]*dplv1.Deployable) {
	if dplmap == nil {
		return
	}
	//If the dpl changes its channel, delete all other children of the dpl who were propagated to other channels before.
	for chstr, dpl := range dplmap {
		r.Log.Info(fmt.Sprintf("deleting deployable %v/%v from channel %v ", dpl.Namespace, dpl.Name, chstr))

		err := r.Client.Delete(context.TODO(), dpl)
		if err != nil {
			klog.Errorf("Failed to delete %v due to %v ", dpl.Name, err)
		}

		//record events
		dplkey := types.NamespacedName{Name: dpl.GetName(), Namespace: dpl.GetNamespace()}
		addtionalMsg := "Depolyable " + dplkey.String() + " removed from the channel"

		dplchannel := &chv1.Channel{}

		parsedstr := strings.Split(chstr, "/")
		if len(parsedstr) != 2 {
			klog.Info("invalid channel namespacedName: ", chstr)

			continue
		}

		chkey := types.NamespacedName{Name: parsedstr[1], Namespace: parsedstr[0]}

		error1 := r.Get(context.TODO(), chkey, dplchannel)
		if error1 != nil {
			r.Log.Info(fmt.Sprintf("Channel %#v not found, unable to record events to it. ", chkey))
		} else {
			err = r.appendEvent(dplchannel, dplkey, err, "Delete", addtionalMsg)
			if err != nil {
				r.Log.Error(err, fmt.Sprintf("Failed to record event %v due to %v ", dplkey, err))
			}
		}
	}
}

func (r *ReconcileDeployable) propagateDeployableToChannel(deployable *dplv1.Deployable,
	dplmap map[string]*dplv1.Deployable, channel *chv1.Channel) error {

	r.Log.Info("enter propagateDeployableToChannel")
	defer r.Log.Info("exit propagateDeployableToChannel")
	chkey := types.NamespacedName{Name: channel.Name, Namespace: channel.Namespace}.String()

	if deployable.Namespace == channel.Namespace {
		r.Log.Info(fmt.Sprintf("The deployable: %#v exists in channel: %#v.", deployable.GetName(), channel.GetName()))

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
		r.Log.Info(fmt.Sprintf("The deployable %#v can't be promoted to channel %#v.", deployable.GetName(), channel.GetName()))
		return nil
	}

	chdpl, err := utils.GenerateDeployableForChannel(deployable, types.NamespacedName{Name: channel.Name, Namespace: channel.Namespace})
	if err != nil {
		return err
	}

	exdpl, ok := dplmap[chkey]

	if !ok {
		r.Log.Info(fmt.Sprintf("Creating deployable in channel", *chdpl))
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

	if reflect.DeepEqual(exdpl.GetAnnotations(), chdpl.GetAnnotations()) &&
		reflect.DeepEqual(exdpl.GetLabels(), chdpl.GetLabels()) &&
		reflect.DeepEqual(exdpl.Spec, chdpl.Spec) {
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

func (r *ReconcileDeployable) updateDeployableRelationWithChannel(
	instance *dplv1.Deployable, dplmap map[string]*dplv1.Deployable,
	parent *dplv1.Deployable, channelNsMap map[string]string,
	channelmap map[string]*chv1.Channel) (map[string]*dplv1.Deployable, error) {
	if len(instance.GetFinalizers()) == 0 {
		annotations := instance.Annotations
		if channelNsMap[instance.Namespace] != "" && annotations != nil && annotations[chv1.KeyChannelSource] != "" && parent == nil {
			r.Log.Info(fmt.Sprintf("Delete instance: The parent of the instance not found: %#v, %#v", annotations[chv1.KeyChannelSource], instance))
			return nil, r.Client.Delete(context.TODO(), instance)
		}

		for _, chname := range instance.Spec.Channels {
			ch, ok := channelmap[chname]
			if !ok {
				r.Log.Info(fmt.Sprintf("failed to find channel name %v for deployable %v/%v", chname, instance.Namespace, instance.Name))
				continue
			}

			if err := r.propagateDeployableToChannel(instance, dplmap, ch); err != nil {
				r.Log.Info(fmt.Sprintf("failed to validate deplyable for %v ", instance))
			}

			delete(channelmap, chname)
		}

		for _, ch := range channelmap {
			if err := r.propagateDeployableToChannel(instance, dplmap, ch); err != nil {
				r.Log.Error(err, fmt.Sprintf("Failed to propagate %v To Channel", instance))
			}
		}
	}

	return dplmap, nil
}
