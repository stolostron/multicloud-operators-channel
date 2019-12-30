// Licensed Materials - Property of IBM
// (c) Copyright IBM Corporation 2016, 2019. All Rights Reserved.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP  Schedule Contract with IBM Corp.

package utils

import (
	"context"

	"github.com/golang/glog"
	appv1alpha1 "github.com/IBM/multicloud-operators-channel/pkg/apis/app/v1alpha1"
	dplv1alpha1 "github.com/IBM/multicloud-operators-deployable/pkg/apis/app/v1alpha1"
	dplutils "github.com/IBM/multicloud-operators-deployable/pkg/utils"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ValidateDeployableInChannel check if a deployable rightfully in channel
func ValidateDeployableInChannel(deployable *dplv1alpha1.Deployable, channel *appv1alpha1.Channel) bool {
	if glog.V(10) {
		fnName := dplutils.GetFnName()
		glog.Infof("Entering: %v()", fnName)
		defer glog.Infof("Exiting: %v()", fnName)
	}
	if deployable == nil || channel == nil {
		return false
	}

	if deployable.Namespace != channel.Namespace {
		return false
	}

	if channel.Spec.Gates == nil {
		return true
	}

	if channel.Spec.Gates.Annotations != nil {
		dplanno := deployable.Annotations
		if dplanno == nil {
			return false
		}

		for k, v := range channel.Spec.Gates.Annotations {
			if dplanno[k] != v {
				return false
			}
		}
	}

	return true
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
func ValidateDeployableToChannel(deployable *dplv1alpha1.Deployable, channel *appv1alpha1.Channel) bool {
	if glog.V(10) {
		fnName := dplutils.GetFnName()
		glog.Infof("Entering: %v()", fnName)
		defer glog.Infof("Exiting: %v()", fnName)
	}
	found := false
	if deployable.Spec.Channels != nil {
		for _, chns := range deployable.Spec.Channels {
			if chns == channel.Name {
				found = true
			}
		}
	}

	if !found {
		if channel.Spec.Gates == nil {
			return false
		}
		if channel.Spec.SourceNamespaces != nil {
			for _, ns := range channel.Spec.SourceNamespaces {
				if ns == deployable.Namespace {
					found = true
				}
			}
		}
	}
	if !found {
		return false
	}

	if channel.Spec.Gates == nil {
		return true
	}

	if channel.Spec.Gates.Annotations != nil {
		dplanno := deployable.Annotations
		if dplanno == nil {
			return false
		}

		for k, v := range channel.Spec.Gates.Annotations {
			if dplanno[k] != v {
				return false
			}
		}
	}

	return true
}

// FindDeployableForChannelsInMap check all deployables in certain namespace delete all has the channel set the given channel namespace
// channelnsMap is a set(), which is used to check up if the dpl is within a channel or not
func FindDeployableForChannelsInMap(cl client.Client, deployable *dplv1alpha1.Deployable, channelnsMap map[string]string) (*dplv1alpha1.Deployable, map[string]*dplv1alpha1.Deployable, error) {
	if glog.V(10) {
		fnName := dplutils.GetFnName()
		glog.Infof("Entering: %v()", fnName)
		defer glog.Infof("Exiting: %v()", fnName)
	}
	if channelnsMap == nil || len(channelnsMap) == 0 {
		return nil, nil, nil
	}

	var parent *dplv1alpha1.Deployable
	dpllist := &dplv1alpha1.DeployableList{}
	err := cl.List(context.TODO(), &client.ListOptions{}, dpllist)
	if err != nil {
		glog.Error("Failed to list deployable for deployable ", *deployable)
		return nil, nil, err
	}

	dplmap := make(map[string]*dplv1alpha1.Deployable)

	//cur dpl key
	dplkey := types.NamespacedName{Name: deployable.Name, Namespace: deployable.Namespace}

	parentkey := ""
	annotations := deployable.GetAnnotations()
	if annotations != nil {
		parentkey = annotations[appv1alpha1.KeyChannelSource]
	}

	parentDplGen := DplGenerateNameStr(deployable)
	glog.Infof("dplkey: %v", dplkey)
	for _, dpl := range dpllist.Items {
		key := types.NamespacedName{Name: dpl.Name, Namespace: dpl.Namespace}.String()
		if key == parentkey {
			parent = dpl.DeepCopy()
		}

		glog.V(10).Infof("parent dpl: %v, checking dpl: %v", deployable.GetName(), dpl.GetGenerateName())
		if dpl.GetGenerateName() == parentDplGen && channelnsMap[dpl.Namespace] != "" {

			dplanno := dpl.GetAnnotations()
			if dplanno != nil && dplanno[appv1alpha1.KeyChannelSource] == dplkey.String() {
				glog.V(10).Infof("adding dpl: %v to children dpl map", dplkey.String())
				dplmap[dplanno[appv1alpha1.KeyChannel]] = dpl.DeepCopy()
			}

		}
	}

	dplmapStr := ""
	for ch, dpl := range dplmap {
		dplmapStr = dplmapStr + "(ch: " + ch + " dpl: " + dpl.GetNamespace() + "/" + dpl.GetName() + ") "
	}

	if parent != nil {
		glog.V(10).Infof("deployable: %#v/%#v, parent: %#v/%#v, dplmap: %#v", deployable.GetNamespace(), deployable.GetName(), parent.GetNamespace(), parent.GetName(), dplmapStr)
	} else {
		glog.V(10).Infof("deployable: %#v/%#v, parent: %#v, dplmap: %#v", deployable.GetNamespace(), deployable.GetName(), parent, dplmapStr)
	}

	return parent, dplmap, nil
}

// CleanupDeployables check all deployables in certain namespace delete all has the channel set the given channel name
func CleanupDeployables(cl client.Client, channel types.NamespacedName) error {
	if glog.V(10) {
		fnName := dplutils.GetFnName()
		glog.Infof("Entering: %v()", fnName)
		defer glog.Infof("Exiting: %v()", fnName)
	}
	dpllist := &dplv1alpha1.DeployableList{}
	err := cl.List(context.TODO(), &client.ListOptions{Namespace: channel.Namespace}, dpllist)
	if err != nil {
		glog.Error("Failed to list deployable for channel namespace ", channel.Namespace)
		return err
	}

	for _, dpl := range dpllist.Items {
		if dpl.Spec.Channels != nil {
			for _, chname := range dpl.Spec.Channels {
				if chname == channel.Name {
					err = cl.Delete(context.TODO(), &dpl)
				}
			}
		}
	}

	return err
}

// GenerateDeployableForChannel generate a copy of deployable for channel with label, annotation, template and channel info
func GenerateDeployableForChannel(deployable *dplv1alpha1.Deployable, channel types.NamespacedName) (*dplv1alpha1.Deployable, error) {
	if glog.V(10) {
		fnName := dplutils.GetFnName()
		glog.Infof("Entering: %v()", fnName)
		defer glog.Infof("Exiting: %v()", fnName)
	}
	if deployable == nil {
		return nil, nil
	}

	chdpl := &dplv1alpha1.Deployable{}

	chdpl.GenerateName = DplGenerateNameStr(deployable)

	chdpl.Namespace = channel.Namespace

	deployable.Spec.DeepCopyInto(&(chdpl.Spec))
	chdpl.Spec.Placement = nil
	chdpl.Spec.Overrides = nil
	chdpl.Spec.Channels = nil
	chdpl.Spec.Dependencies = nil

	labels := deployable.GetLabels()
	if len(labels) > 0 {
		chdpllabels := make(map[string]string)
		for k, v := range labels {
			chdpllabels[k] = v
		}
		chdpl.SetLabels(chdpllabels)
	}

	chsrc := types.NamespacedName{Name: deployable.Name, Namespace: deployable.Namespace}.String()
	annotations := deployable.GetAnnotations()
	chdplannotations := make(map[string]string)
	if len(annotations) > 0 {
		for k, v := range annotations {
			chdplannotations[k] = v
		}
		if chdplannotations[appv1alpha1.KeyChannelSource] != "" {
			chsrc = chdplannotations[appv1alpha1.KeyChannelSource]
		}
	}
	chdplannotations[dplv1alpha1.AnnotationLocal] = "false"
	chdplannotations[appv1alpha1.KeyChannelSource] = chsrc
	chdplannotations[appv1alpha1.KeyChannel] = types.NamespacedName{Name: channel.Name, Namespace: channel.Namespace}.String()
	chdplannotations[dplv1alpha1.AnnotationIsGenerated] = "true"

	if v, ok := annotations[dplv1alpha1.AnnotationDeployableVersion]; ok {
		chdplannotations[dplv1alpha1.AnnotationDeployableVersion] = v
	}

	chdpl.SetAnnotations(chdplannotations)

	return chdpl, nil
}

//DplGenerateNameStr  will generate a string for the dpl generate name
func DplGenerateNameStr(deployable *dplv1alpha1.Deployable) string {
	if glog.V(10) {
		fnName := dplutils.GetFnName()
		glog.Infof("Entering: %v()", fnName)
		defer glog.Infof("Exiting: %v()", fnName)
	}
	gn := ""
	if deployable.GetGenerateName() == "" {
		gn = deployable.GetName() + "-"
	} else {
		gn = deployable.GetGenerateName() + "-"
	}
	return gn
}
