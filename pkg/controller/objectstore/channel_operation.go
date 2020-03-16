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
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/pkg/errors"

	"github.com/ghodss/yaml"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"
	"github.com/open-cluster-management/multicloud-operators-channel/pkg/utils"
	dplv1 "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis/apps/v1"
)

const ChnNumPerNamespace = 1

func isObjectGenerateByHub(objtpl *unstructured.Unstructured) bool {
	tplannotations := objtpl.GetAnnotations()
	if tplannotations == nil {
		return false
	}

	if _, ok := tplannotations[dplv1.AnnotationHosting]; !ok {
		return false
	}

	return true
}

// ReconcileForChannel populate object store with channel when turned on
func (r *ReconcileDeployable) deleteDeployableInObjectBucket(request types.NamespacedName) error {
	dplchn, ok := r.isReconileSignalLinkToChannel(request.Namespace)
	if !ok {
		klog.Infof("skip reconcile, deployable not sitting in a channel")
		return nil
	}

	chndesc, err := r.findObjectBucketForDeployable(dplchn)
	if err != nil {
		return errors.Wrap(err, "failed to bucket configured for deployable")
	}

	//ignoring deployable generate name, since channel is managing deployable via deployable name
	dplObj, err := chndesc.ObjectStore.Get(chndesc.Bucket, request.Name)
	if err != nil {
		return errors.Wrapf(err, "failed to get object %v/%v, it might be delete ready, err: %v ", chndesc.Bucket, request.Name, err)
	}

	objtpl := &unstructured.Unstructured{}

	err = yaml.Unmarshal(dplObj.Content, objtpl)
	if err != nil {
		return errors.Wrapf(err, "failed to unmashall %v/%v", chndesc.Bucket, request.Name)
	}

	// Only delete templates created and uploaded to objectstore by deployables Reconcile
	// meaning AnnotationHosting must be set in their template
	if !isObjectGenerateByHub(objtpl) {
		klog.Infof("skip delete deployable %v/%v, not created by a hub deployable", objtpl.GetName(), objtpl.GetNamespace())
		return nil
	}

	klog.Info("Deleting ", chndesc.Bucket, request.Name)

	if err := chndesc.ObjectStore.Delete(chndesc.Bucket, request.Name); err != nil {
		return errors.Wrapf(err, "failed to delete object %v from bucket %v", chndesc.Bucket, request.Name)
	}

	return nil
}

func (r *ReconcileDeployable) isReconileSignalLinkToChannel(reqNs string) (*chv1.Channel, bool) {
	dplchn := r.getChannelForNamespace(reqNs)

	if dplchn == nil {
		return nil, false
	}

	return dplchn, true
}

func (r *ReconcileDeployable) findObjectBucketForDeployable(dplchn *chv1.Channel) (*utils.ChannelDescription, error) {
	if err := r.makeConnectToBucket(dplchn); err != nil {
		return nil, errors.Wrap(err, "faild to find the channel from descriptor map")
	}

	chndesc, ok := r.ChannelDescriptor.Get(dplchn.Name)
	if !ok {
		return nil, errors.New("failed to get deployable from object bucket")
	}

	return chndesc, nil
}

// reconcileForChannel populate object store with channel when turned on
func (r *ReconcileDeployable) createOrUpdateDeployableInObjectBucket(deployable *dplv1.Deployable) error {
	dplchn, ok := r.isReconileSignalLinkToChannel(deployable.GetNamespace())
	if !ok {
		klog.Infof("skip reconcile, deployable not sitting in a channel")
		return nil
	}

	chndesc, err := r.findObjectBucketForDeployable(dplchn)
	if err != nil {
		return errors.Wrapf(err, "failed to configure bucket for deployable %v/%v", deployable.Namespace, deployable.Name)
	}

	template, err := prepareDeployalbeTemplate(deployable)
	if err != nil {
		return errors.Wrap(err, "failed to handle deployable.spec.temaplate")
	}

	tplb, err := yaml.Marshal(template)
	if err != nil {
		return errors.Wrap(err, "failed to marshall packaged deployable")
	}

	var dplGenerateName string

	if deployable.GetGenerateName() != "" {
		dplGenerateName = deployable.GetGenerateName()
	} else {
		dplGenerateName = deployable.GetName()
	}

	dplObj := utils.DeployableObject{
		Name:         deployable.GetName(),
		GenerateName: dplGenerateName,
		Content:      tplb,
		Version:      deployable.GetAnnotations()[dplv1.AnnotationDeployableVersion],
	}

	if err := chndesc.ObjectStore.Put(chndesc.Bucket, dplObj); err != nil {
		return errors.Wrap(err, "failed to put to object bucket")
	}

	return nil
}

func (r *ReconcileDeployable) getChannelForNamespace(namespace string) *chv1.Channel {
	dplchnlist := &chv1.ChannelList{}

	err := r.KubeClient.List(context.TODO(), dplchnlist, &client.ListOptions{Namespace: namespace})
	if err != nil {
		klog.Errorf("failed to find deployable from %v, err: %+v", namespace, err)
		return nil
	}

	if len(dplchnlist.Items) != ChnNumPerNamespace {
		klog.Errorf("incorrect channel setting %v namespace, itmes %v", namespace, dplchnlist.Items)
		return nil
	}

	if !strings.EqualFold(string(dplchnlist.Items[0].Spec.Type), chv1.ChannelTypeObjectBucket) {
		klog.Error("wrong channel type")
		return nil
	}

	dplchn := dplchnlist.Items[0].DeepCopy()

	return dplchn
}

func (r *ReconcileDeployable) makeConnectToBucket(dplchn *chv1.Channel) error {
	_, ok := r.ChannelDescriptor.Get(dplchn.Name)
	if !ok {
		klog.Info("Syncing channel ", dplchn.Name)

		if err := r.deleteOrUpdateBucketWithDeployablesInChannel(dplchn); err != nil {
			return errors.Wrap(err, fmt.Sprintf("Failed to sync channel %v", dplchn.Name))
		}
	} else if err := r.ChannelDescriptor.ConnectWithResourceHost(dplchn, r.KubeClient); err != nil {
		return errors.Wrap(err, fmt.Sprintf("Failed to validate channel %v", dplchn.Name))
	}

	return nil
}

// sync channel info with channel namespace. For ObjectBucket channel, namespace is source of truth
// WARNNING: if channel is deleted during controller outage, bucket won't be cleaned up
func (r *ReconcileDeployable) deleteOrUpdateBucketWithDeployablesInChannel(dplchn *chv1.Channel) error {
	if err := r.ChannelDescriptor.ConnectWithResourceHost(dplchn, r.KubeClient); err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to validate channel %v", dplchn.Name))
	}

	chndesc, ok := r.ChannelDescriptor.Get(dplchn.Name)
	if !ok {
		return errors.New(fmt.Sprintf("failed to get channel description for %v ", dplchn.Name))
	}

	dpllist := &dplv1.DeployableList{}

	if err := r.KubeClient.List(context.TODO(), dpllist, &client.ListOptions{Namespace: dplchn.GetNamespace()}); err != nil {
		return errors.Wrap(err, "failed to list deployables")
	}

	chndplmap := make(map[string]*dplv1.Deployable)
	for _, dpl := range dpllist.Items {
		chndplmap[dpl.Name] = dpl.DeepCopy()
	}

	objnames, err := chndesc.List(chndesc.Bucket)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to list all objects in bucket %v ", chndesc.Bucket))
	}

	for _, name := range objnames {
		dplObj, err := chndesc.ObjectStore.Get(chndesc.Bucket, name)
		if err != nil {
			klog.Error("Failed to get object ", chndesc.Bucket, "/", name, " err:", err)
			continue
		}

		objtpl := &unstructured.Unstructured{}

		if err := yaml.Unmarshal(dplObj.Content, objtpl); err != nil {
			klog.Error("Failed to get object ", chndesc.Bucket, "/", name, " err:", err)
			continue
		}

		if !isObjectGenerateByHub(objtpl) {
			continue
		}

		// Delete templates that don't exist in the channel namespace anymore
		if _, ok := chndplmap[name]; !ok {
			err = chndesc.ObjectStore.Delete(chndesc.Bucket, name)
			if err != nil {
				klog.Error("Failed to delete ", chndesc.Bucket, "/", name, " err:", err)
			}

			continue
		}

		// Update templates if they are updated in the channel namespace
		dpl := chndplmap[name]

		dpltpl, err := prepareDeployalbeTemplate(dpl)
		if err != nil {
			continue
		}

		if !reflect.DeepEqual(objtpl, dpltpl) {
			dplb, err := yaml.Marshal(dpltpl)
			if err != nil {
				klog.Error("YAML unMashall ", dpl, " err:", err)
				continue
			}

			dplObj.Content = dplb

			dpltplannotations := dpltpl.GetAnnotations()
			dplObj.Version = dpltplannotations[dplv1.AnnotationDeployableVersion]

			if err := chndesc.ObjectStore.Put(chndesc.Bucket, dplObj); err != nil {
				klog.Error("Failed to Put", chndesc.Bucket, "/", name, " err:", err)
			}
		}
	}

	return nil
}

func prepareDeployalbeTemplate(dpl *dplv1.Deployable) (*unstructured.Unstructured, error) {
	dpltpl := &unstructured.Unstructured{}

	if dpl.Spec.Template == nil {
		klog.Warning("Processing deployable without template:", dpl)
		return dpltpl, errors.New(fmt.Sprintf("processing deployable %v without template", dpl))
	}

	if err := json.Unmarshal(dpl.Spec.Template.Raw, dpltpl); err != nil {
		return dpltpl, errors.New(fmt.Sprintf("failed to unmashall deployable %v, err: %v", dpl, err))
	}

	// Update template annotations from deployable annotations
	dpltplannotations := dpltpl.GetAnnotations()
	if dpltplannotations == nil {
		dpltplannotations = make(map[string]string)
	}

	for k, v := range dpl.GetAnnotations() {
		dpltplannotations[k] = v
	}

	hosting := types.NamespacedName{Name: dpl.GetName(), Namespace: dpl.GetNamespace()}.String()
	dpltplannotations[dplv1.AnnotationHosting] = hosting
	dpltpl.SetAnnotations(dpltplannotations)

	// Update template labels from deployable labels
	labels := dpl.GetLabels()
	if len(labels) > 0 {
		tpllbls := dpltpl.GetLabels()
		if tpllbls == nil {
			tpllbls = make(map[string]string)
		}

		for k, v := range labels {
			tpllbls[k] = v
		}

		dpltpl.SetLabels(tpllbls)
	}

	return dpltpl, nil
}
