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
	helmsync "github.com/open-cluster-management/multicloud-operators-channel/pkg/synchronizer/helmreposynchronizer"
	"github.com/open-cluster-management/multicloud-operators-channel/pkg/utils"
	dplv1 "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis/apps/v1"

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

const (
	controllerName                = "deployable"
	controllerSetup               = "deployable-setup"
	CtrlDeployableIndexer         = "origin-deployable"
	CtrlGenerateDeployableIndexer = "generated-deployable"
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new Channel Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, recorder record.EventRecorder, logger logr.Logger,
	channelDescriptor *utils.ChannelDescriptor, sync *helmsync.ChannelSynchronizer) error {
	return add(mgr, newReconciler(mgr, recorder, logger.WithName(controllerName)), logger.WithName(controllerSetup))
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
	if err := mapper.List(
		context.TODO(),
		dpllist,
		client.MatchingFields{CtrlDeployableIndexer: "true"},
	); err != nil {
		mapper.log.Error(err, "failed to list all deployable ")
		return nil
	}

	objKey := types.NamespacedName{Name: obj.Meta.GetName(), Namespace: obj.Meta.GetNamespace()}
	mapper.log.Info(fmt.Sprintf("channel %v mapper's dpl list %v\n", objKey.String(), len(dpllist.Items)))

	var requests []reconcile.Request

	for _, dpl := range dpllist.Items {
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Name: dpl.GetName(), Namespace: dpl.GetNamespace()}})
	}

	return requests
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler, logger logr.Logger) error {
	if err := mgr.GetFieldIndexer().IndexField(context.TODO(), &dplv1.Deployable{}, CtrlDeployableIndexer, func(rawObj runtime.Object) []string {
		// grab the job object, extract the owner...
		dpl := rawObj.(*dplv1.Deployable)
		anno := dpl.GetAnnotations()
		if len(anno) == 0 {
			return nil
		}
		// this make sure the indexer will only be applied on non-generated
		// deployable
		if _, ok := anno[chv1.KeyChannelSource]; ok {
			return nil
		}

		// ...and if so, return it
		return []string{"true"}
	}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(context.TODO(), &dplv1.Deployable{}, CtrlGenerateDeployableIndexer, func(rawObj runtime.Object) []string {
		// grab the job object, extract the owner...
		dpl := rawObj.(*dplv1.Deployable)
		anno := dpl.GetAnnotations()
		if len(anno) == 0 {
			return nil
		}
		// this make sure the indexer will only be applied on generated
		// deployable
		if _, ok := anno[chv1.KeyChannelSource]; !ok {
			return nil
		}

		// ...and if so, return it
		return []string{"true"}
	}); err != nil {
		return err
	}
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
// this reconcile will be triggered when deployable or channel resources is changed on kube-apiserver.
// this reconcile will do the following things,
// 1. promote deployable from a channel's target namespace to channel's namespace(also, do the delete as well)
// 1.1 channel type should be namespace or object store

// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.open-cluster-management.io,resources=channels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.open-cluster-management.io,resources=channels/status,verbs=get;update;patch
func (r *ReconcileDeployable) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	log := r.Log.WithValues("dpl-reconcile", request.NamespacedName)
	log.Info(fmt.Sprintf("Starting %v reconcile loop for %v", controllerName, request.NamespacedName))

	defer log.Info(fmt.Sprintf("Finish %v reconcile loop for %v", controllerName, request.NamespacedName))

	dpl := &dplv1.Deployable{}
	err := r.Get(context.TODO(), request.NamespacedName, dpl)

	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			log.Info("exit, deployables will be GC'ed by k8s")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	clusterChnMap, err := utils.GenerateChannelMap(r.Client, log)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Error(err, "failed to get all deployables")
			return reconcile.Result{}, nil
		}

		clusterChnMap = make(map[string]*chv1.Channel)
	}

	channelNsMap := make(map[string]string)

	for _, ch := range clusterChnMap {
		channelNsMap[ch.Namespace] = ch.Name
	}

	parent, childDplmap, err := utils.RebuildDeployableRelationshipGraph(r.Client, dpl, channelNsMap, log)
	if err != nil && !errors.IsNotFound(err) {
		log.Error(err, "failed to get all deployables")
		return reconcile.Result{}, nil
	}

	log.Info(fmt.Sprintf("Dpl Map, before deletion: %#v", childDplmap))

	childDplmap, err = r.promoteDeployabeToChannels(dpl, childDplmap, parent, channelNsMap, clusterChnMap, log)
	if err != nil {
		return reconcile.Result{}, err
	}

	r.handleOrphanDeployable(childDplmap, log)

	return reconcile.Result{}, err
}

func (r *ReconcileDeployable) handleOrphanDeployable(dplmap map[string]*dplv1.Deployable, logger logr.Logger) {
	if dplmap == nil {
		return
	}
	//If the dpl changes its channel, delete all other children of the dpl who were propagated to other channels before.
	for chstr, dpl := range dplmap {
		logger.Info(fmt.Sprintf("deleting deployable %v/%v from channel %v ", dpl.Namespace, dpl.Name, chstr))

		err := r.Client.Delete(context.TODO(), dpl)
		if err != nil {
			logger.Error(err, fmt.Sprintf("Failed to delete %v ", dpl.Name))
		}

		//record events
		dplkey := types.NamespacedName{Name: dpl.GetName(), Namespace: dpl.GetNamespace()}
		addtionalMsg := "Depolyable " + dplkey.String() + " removed from the channel"

		dplchannel := &chv1.Channel{}

		parsedstr := strings.Split(chstr, "/")
		if len(parsedstr) != 2 {
			logger.Info(fmt.Sprintf("invalid channel namespacedName: %v", chstr))
			continue
		}

		chkey := types.NamespacedName{Name: parsedstr[1], Namespace: parsedstr[0]}

		error1 := r.Get(context.TODO(), chkey, dplchannel)
		if error1 != nil {
			logger.Info(fmt.Sprintf("Channel %#v not found, unable to record events to it. ", chkey))
		} else {
			err = r.appendEvent(dplchannel, dplkey, err, "Delete", addtionalMsg)
			if err != nil {
				logger.Error(err, fmt.Sprintf("Failed to record event %v", dplkey))
			}
		}
	}
}

func (r *ReconcileDeployable) propagateDeployableToChannel(
	deployable *dplv1.Deployable, dplmap map[string]*dplv1.Deployable,
	channel *chv1.Channel, logger logr.Logger) error {
	logger.Info("enter propagateDeployableToChannel")

	defer logger.Info("exit propagateDeployableToChannel")

	chkey := types.NamespacedName{Name: channel.Name, Namespace: channel.Namespace}.String()

	if deployable.Namespace == channel.Namespace {
		logger.Info(fmt.Sprintf("The deployable: %#v exists in channel: %#v.", deployable.GetName(), channel.GetName()))

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
	if channelHasDeployable(r.Client, channel, deployable) {
		logger.Info(fmt.Sprintf("The generated deployable %#v already exist in channel %#v.", deployable.GetName(), channel.GetName()))
		return nil
	}

	if !utils.ValidateDeployableToChannel(deployable, channel) {
		logger.Info(fmt.Sprintf("The deployable %#v can't be promoted to channel %#v.", deployable.GetName(), channel.GetName()))
		return nil
	}

	chdpl, err := utils.GenerateDeployableForChannel(deployable, types.NamespacedName{Name: channel.Name, Namespace: channel.Namespace})
	if err != nil {
		return err
	}

	// for hub subscription to get its subscribing resource
	addL := map[string]string{
		chv1.KeyChannel:     channel.GetName(),
		chv1.KeyChannelType: string(channel.Spec.Type),
	}

	utils.AddOrAppendChannelLabel(chdpl, addL)

	exdpl, ok := dplmap[chkey]

	if !ok {
		logger.Info(fmt.Sprintf("Creating deployable in channel %v", *chdpl))
		err = r.Client.Create(context.TODO(), chdpl)

		//record events
		dplkey := types.NamespacedName{Name: chdpl.GetName(), Namespace: chdpl.GetNamespace()}
		addtionalMsg := "Depolyable " + dplkey.String() + " created in the channel"

		err = r.appendEvent(channel, dplkey, err, "Deploy", addtionalMsg)
		if err != nil {
			logger.Error(err, fmt.Sprintf("failed to record event %v", dplkey))
		}

		return err
	}

	if reflect.DeepEqual(exdpl.GetAnnotations(), chdpl.GetAnnotations()) &&
		reflect.DeepEqual(exdpl.GetLabels(), chdpl.GetLabels()) &&
		reflect.DeepEqual(exdpl.Spec, chdpl.Spec) {
		logger.Info(fmt.Sprintf("No changes to existing deployable in channel %v ", types.NamespacedName{Name: exdpl.GetName(), Namespace: exdpl.GetNamespace()}))
	} else {
		exdpl.SetLabels(chdpl.GetLabels())
		exdpl.SetAnnotations(chdpl.GetAnnotations())
		chdpl.Spec.DeepCopyInto(&(exdpl.Spec))
		err = r.Client.Update(context.TODO(), exdpl)
		logger.Info(fmt.Sprintf("Updating existing deployable in channel to %v", *exdpl))

		//record events
		dplkey := types.NamespacedName{Name: exdpl.GetName(), Namespace: exdpl.GetNamespace()}
		addtionalMsg := "Depolyable " + dplkey.String() + " updated in the channel"

		err = r.appendEvent(channel, dplkey, err, "Deploy", addtionalMsg)
		if err != nil {
			logger.Error(err, fmt.Sprintf("failed to record event %v", dplkey))
		}
	}

	delete(dplmap, chkey)

	return err
}

func (r *ReconcileDeployable) promoteDeployabeToChannels(
	dpl *dplv1.Deployable, childDplMap map[string]*dplv1.Deployable,
	parent *dplv1.Deployable, chNsSet map[string]string,
	clusterChnMap map[string]*chv1.Channel, logger logr.Logger) (map[string]*dplv1.Deployable, error) {
	if len(dpl.GetFinalizers()) == 0 {
		annotations := dpl.Annotations
		if chNsSet[dpl.Namespace] != "" && annotations != nil && annotations[chv1.KeyChannelSource] != "" && parent == nil {
			r.Log.Info(fmt.Sprintf("Delete instance: The parent of the instance not found: %#v, %#v", annotations[chv1.KeyChannelSource], dpl))
			return nil, r.Client.Delete(context.TODO(), dpl)
		}

		logger.Info(fmt.Sprintf("cluster has channels: %#v", clusterChnMap))
		//promote deployable to channels, specified in deployable.Spec.Channels
		for _, chname := range dpl.Spec.Channels {
			ch, ok := clusterChnMap[chname]
			if !ok {
				logger.Info(fmt.Sprintf("failed to find channel name %v for deployable %v/%v", chname, dpl.Namespace, dpl.Name))
				continue
			}

			if err := r.propagateDeployableToChannel(dpl, childDplMap, ch, logger); err != nil {
				logger.Info(fmt.Sprintf("failed to validate deplyable for %v ", dpl))
			}

			delete(clusterChnMap, chname)
		}

		//promote deployable to channels, who's watching the deployable's
		//namespace
		for _, ch := range clusterChnMap {
			if err := r.propagateDeployableToChannel(dpl, childDplMap, ch, logger); err != nil {
				logger.Error(err, fmt.Sprintf("Failed to propagate %v To Channel", dpl))
			}
		}
	}

	return childDplMap, nil
}

func channelHasDeployable(clt client.Client, chn *chv1.Channel, dpl *dplv1.Deployable) bool {
	gn := dpl.GetGenerateName()
	if len(gn) == 0 {
		return false
	}

	chDpls := &dplv1.DeployableList{}

	if err := clt.List(context.TODO(), chDpls, client.InNamespace(chn.GetNamespace())); err != nil {
		return true
	}

	if len(chDpls.Items) == 0 {
		return false
	}

	for _, item := range chDpls.Items {
		if item.GetGenerateName() == gn {
			return true
		}
	}

	return false
}
