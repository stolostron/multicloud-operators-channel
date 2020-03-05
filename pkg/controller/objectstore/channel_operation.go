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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"
	"github.com/open-cluster-management/multicloud-operators-channel/pkg/utils"
	dplv1 "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis/apps/v1"
)

//

// ReconcileForChannel populate object store with channel when turned on
func (r *ReconcileDeployable) deleteDeployableInObjectStore(request types.NamespacedName) (reconcile.Result, error) {
	dplchn, _ := r.getChannelForNamespace(request.Namespace)
	if dplchn == nil {
		return reconcile.Result{}, nil
	}

	err := r.validateChannel(dplchn)
	if err != nil {
		return reconcile.Result{}, nil
	}

	chndesc, ok := r.ChannelDescriptor.Get(dplchn.Name)
	if !ok {
		klog.Info("Failed to get channel description for ", dplchn.Name)
		return reconcile.Result{}, nil
	}

	//ignoring deployable generate name, since channel is managing deployable via deployable name
	dplObj, err := chndesc.ObjectStore.Get(chndesc.Bucket, request.Name)
	if err != nil {
		klog.Error("Failed to get object ", chndesc.Bucket, "/", request.Name, " err:", err)
		return reconcile.Result{}, err
	}

	objtpl := &unstructured.Unstructured{}

	err = yaml.Unmarshal(dplObj.Content, objtpl)
	if err != nil {
		klog.Error("Failed to unmashall ", chndesc.Bucket, "/", request.Name, " err:", err)
		return reconcile.Result{}, err
	}

	// Only delete templates created and uploaded to objectstore by deployables Reconcile
	// meaning AnnotationHosting must be set in their template
	tplannotations := objtpl.GetAnnotations()

	if tplannotations == nil {
		return reconcile.Result{}, nil
	}

	if _, ok := tplannotations[dplv1.AnnotationHosting]; !ok {
		return reconcile.Result{}, nil
	}

	klog.Info("Deleting ", chndesc.Bucket, request.Name)

	err = chndesc.ObjectStore.Delete(chndesc.Bucket, request.Name)

	return reconcile.Result{}, err
}

// reconcileForChannel populate object store with channel when turned on
func (r *ReconcileDeployable) reconcileForChannel(deployable *dplv1.Deployable) (reconcile.Result, error) {
	dplchn, err := r.getChannelForNamespace(deployable.Namespace)
	if dplchn == nil || err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to find channel deployables")
	}

	err = r.validateChannel(dplchn)
	if err != nil {
		return reconcile.Result{}, err
	}

	chndesc, ok := r.ChannelDescriptor.Get(dplchn.Name)
	if !ok {
		return reconcile.Result{}, errors.New("failed to get deployable from object bucket")
	}

	template := &unstructured.Unstructured{}

	if deployable.Spec.Template == nil {
		klog.Warning("Processing deployable without template:", deployable)
		return reconcile.Result{}, errors.New(fmt.Sprintf("skip deployables with empty template %v", deployable))
	}

	err = json.Unmarshal(deployable.Spec.Template.Raw, template)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, fmt.Sprintf("failed to unmarshal template %v", deployable.Spec.Template.Raw))
	}

	// Only reconcile templates that are not created by objectstore synchronizer
	// meaning AnnotationExternalSource is not set in their template
	annotations := deployable.GetAnnotations()

	tplannotations := template.GetAnnotations()
	if tplannotations == nil {
		tplannotations = make(map[string]string)
	} else if _, ok := tplannotations[dplv1.AnnotationExternalSource]; ok {
		return reconcile.Result{}, errors.New("skip external deployable")
	}
	// carry deployable annotations
	for k, v := range annotations {
		tplannotations[k] = v
	}
	// Set AnnotationHosting
	hosting := types.NamespacedName{Name: deployable.GetName(), Namespace: deployable.GetNamespace()}.String()
	tplannotations[dplv1.AnnotationHosting] = hosting
	template.SetAnnotations(tplannotations)

	// carry deployable labels
	labels := deployable.GetLabels()
	if len(labels) > 0 {
		tpllbls := template.GetLabels()
		if tpllbls == nil {
			tpllbls = make(map[string]string)
		}

		for k, v := range labels {
			tpllbls[k] = v
		}

		template.SetLabels(tpllbls)
	}

	tplb, err := yaml.Marshal(template)
	if err != nil {
		klog.V(10).Info("YAML marshall ", template, "error:", err)
		return reconcile.Result{}, errors.Wrap(err, "yaml marshall failed")
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
		Version:      annotations[dplv1.AnnotationDeployableVersion],
	}
	if err := chndesc.ObjectStore.Put(chndesc.Bucket, dplObj); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to put to object bucket")
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileDeployable) getChannelForNamespace(namespace string) (*chv1.Channel, error) {
	dplchnlist := &chv1.ChannelList{}

	err := r.KubeClient.List(context.TODO(), dplchnlist, &client.ListOptions{Namespace: namespace})
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to find deployable from %v", namespace))
	}

	if len(dplchnlist.Items) != 1 {
		return nil, errors.Wrap(err, fmt.Sprintf("incorrect channel setting %v namespace, itmes %v", namespace, dplchnlist.Items))
	}

	dplchn := dplchnlist.Items[0].DeepCopy()

	if !strings.EqualFold(string(dplchn.Spec.Type), chv1.ChannelTypeObjectBucket) {
		return nil, errors.New("wrong channel type")
	}

	return dplchn, nil
}

func (r *ReconcileDeployable) validateChannel(dplchn *chv1.Channel) error {
	_, ok := r.ChannelDescriptor.Get(dplchn.Name)
	if !ok {
		klog.Info("Syncing channel ", dplchn.Name)

		err := r.syncChannel(dplchn)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("Failed to sync channel %v", dplchn.Name))
		}
	} else {
		err := r.ChannelDescriptor.ConnectWithResourceHost(dplchn, r.KubeClient)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("Failed to validate channel %v", dplchn.Name))
		}
	}

	return nil
}

// sync channel info with channel namespace. For ObjectBucket channel, namespace is source of truth
// WARNNING: if channel is deleted during controller outage, bucket won't be cleaned up
func (r *ReconcileDeployable) syncChannel(dplchn *chv1.Channel) error {
	err := r.ChannelDescriptor.ConnectWithResourceHost(dplchn, r.KubeClient)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to validate channel %v", dplchn.Name))
	}

	chndesc, ok := r.ChannelDescriptor.Get(dplchn.Name)
	if !ok {
		return errors.New(fmt.Sprintf("failed to get channel description for %v ", dplchn.Name))
	}

	dpllist := &dplv1.DeployableList{}

	err = r.KubeClient.List(context.TODO(), dpllist, &client.ListOptions{Namespace: dplchn.GetNamespace()})
	if err != nil {
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

		if !isValidObj(objtpl) {
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

			err = chndesc.ObjectStore.Put(chndesc.Bucket, dplObj)
			if err != nil {
				klog.Error("Failed to Put", chndesc.Bucket, "/", name, " err:", err)
			}
		}
	}

	return nil
}

func isValidObj(objtpl *unstructured.Unstructured) bool {
	tplannotations := objtpl.GetAnnotations()
	if tplannotations == nil {
		return false
	}

	if _, ok := tplannotations[dplv1.AnnotationHosting]; !ok {
		return false
	}

	return true
}

func prepareDeployalbeTemplate(dpl *dplv1.Deployable) (*unstructured.Unstructured, error) {
	dpltpl := &unstructured.Unstructured{}

	if dpl.Spec.Template == nil {
		klog.Warning("Processing deployable without template:", dpl)
		return dpltpl, errors.New(fmt.Sprintf("processing deployable %v without template", dpl))
	}

	err := json.Unmarshal(dpl.Spec.Template.Raw, dpltpl)

	if err != nil {
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
