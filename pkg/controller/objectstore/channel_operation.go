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
	"reflect"
	"strings"

	"github.com/ghodss/yaml"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	chnv1alpha1 "github.com/IBM/multicloud-operators-channel/pkg/apis/app/v1alpha1"
	"github.com/IBM/multicloud-operators-channel/pkg/utils"
	appv1alpha1 "github.com/IBM/multicloud-operators-deployable/pkg/apis/app/v1alpha1"
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

	if _, ok := tplannotations[appv1alpha1.AnnotationHosting]; !ok {
		return reconcile.Result{}, nil
	}

	klog.Info("Deleting ", chndesc.Bucket, request.Name)

	err = chndesc.ObjectStore.Delete(chndesc.Bucket, request.Name)

	return reconcile.Result{}, err
}

// ReconcileForChannel populate object store with channel when turned on
func (r *ReconcileDeployable) reconcileForChannel(deployable *appv1alpha1.Deployable) (reconcile.Result, error) {
	dplchn, err := r.getChannelForNamespace(deployable.Namespace)
	if dplchn == nil || err != nil {
		return reconcile.Result{}, nil
	}

	err = r.validateChannel(dplchn)
	if err != nil {
		return reconcile.Result{}, nil
	}

	chndesc, ok := r.ChannelDescriptor.Get(dplchn.Name)
	if !ok {
		klog.Info("Failed to get channel description for ", dplchn.Name)
		return reconcile.Result{}, nil
	}

	template := &unstructured.Unstructured{}

	if deployable.Spec.Template == nil {
		klog.Warning("Processing deployable without template:", deployable)
		return reconcile.Result{}, nil
	}

	err = json.Unmarshal(deployable.Spec.Template.Raw, template)
	if err != nil {
		klog.Error("Failed to unmarshal template with error: ", err, " with ", string(deployable.Spec.Template.Raw))
		return reconcile.Result{}, err
	}

	// Only reconcile templates that are not created by objectstore synchronizer
	// meaning AnnotationExternalSource is not set in their template
	annotations := deployable.GetAnnotations()

	tplannotations := template.GetAnnotations()
	if tplannotations == nil {
		tplannotations = make(map[string]string)
	} else if _, ok := tplannotations[appv1alpha1.AnnotationExternalSource]; ok {
		return reconcile.Result{}, nil
	}
	// carry deployable annotations
	for k, v := range annotations {
		tplannotations[k] = v
	}
	// Set AnnotationHosting
	hosting := types.NamespacedName{Name: deployable.GetName(), Namespace: deployable.GetNamespace()}.String()
	tplannotations[appv1alpha1.AnnotationHosting] = hosting
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
		return reconcile.Result{}, err
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
		Version:      annotations[appv1alpha1.AnnotationDeployableVersion],
	}
	err = chndesc.ObjectStore.Put(chndesc.Bucket, dplObj)

	return reconcile.Result{}, err
}

func (r *ReconcileDeployable) getChannelForNamespace(namespace string) (*chnv1alpha1.Channel, error) {
	dplchnlist := &chnv1alpha1.ChannelList{}

	err := r.KubeClient.List(context.TODO(), dplchnlist, &client.ListOptions{Namespace: namespace})
	if err != nil {
		klog.Info("Failed to find channel info in namespace ", namespace, " with error:", err)
		return nil, err
	}

	if len(dplchnlist.Items) != 1 {
		klog.V(10).Info("Incorrect channel setting for namespace:", namespace, " items:", dplchnlist.Items, " It might be ok if it is not a channel namespace")
		return nil, err
	}

	dplchn := dplchnlist.Items[0].DeepCopy()

	if !strings.EqualFold(string(dplchn.Spec.Type), chnv1alpha1.ChannelTypeObjectBucket) {
		return nil, nil
	}

	return dplchn, nil
}

func (r *ReconcileDeployable) validateChannel(dplchn *chnv1alpha1.Channel) error {
	_, ok := r.ChannelDescriptor.Get(dplchn.Name)
	if !ok {
		klog.Info("Syncing channel ", dplchn.Name)

		err := r.syncChannel(dplchn)
		if err != nil {
			klog.Info("Failed to sync channel ", dplchn.Name, " err:", err)
			return err
		}
	} else {
		err := r.ChannelDescriptor.ValidateChannel(dplchn, r.KubeClient)
		if err != nil {
			klog.Info("Failed to validate channel ", dplchn.Name, " err:", err)
			return err
		}
	}

	return nil
}

// sync channel info with channel namespace. For ObjectBucket channel, namespace is source of truth
// WARNNING: if channel is deleted during controller outage, bucket won't be cleaned up
func (r *ReconcileDeployable) syncChannel(dplchn *chnv1alpha1.Channel) error {
	err := r.ChannelDescriptor.ValidateChannel(dplchn, r.KubeClient)
	if err != nil {
		klog.Info("Failed to validate channel ", dplchn.Name, " err:", err)
		return err
	}

	chndesc, ok := r.ChannelDescriptor.Get(dplchn.Name)
	if !ok {
		klog.Info("Failed to get channel description for ", dplchn.Name)
		return nil
	}

	dpllist := &appv1alpha1.DeployableList{}

	err = r.KubeClient.List(context.TODO(), dpllist, &client.ListOptions{Namespace: dplchn.GetNamespace()})
	if err != nil {
		klog.Error("Failed to list all deployable ")
		return nil
	}

	chndplmap := make(map[string]*appv1alpha1.Deployable)
	for _, dpl := range dpllist.Items {
		chndplmap[dpl.Name] = dpl.DeepCopy()
	}

	objnames, err := chndesc.List(chndesc.Bucket)
	if err != nil {
		klog.Error("Failed to list all objects in bucket ", chndesc.Bucket)
		return nil
	}

	for _, name := range objnames {
		dplObj, err := chndesc.ObjectStore.Get(chndesc.Bucket, name)
		if err != nil {
			klog.Error("Failed to get object ", chndesc.Bucket, "/", name, " err:", err)
			continue
		}

		objtpl := &unstructured.Unstructured{}

		err = yaml.Unmarshal(dplObj.Content, objtpl)
		if err != nil {
			klog.Error("Failed to unmashall ", chndesc.Bucket, "/", name, " err:", err)
			continue
		}

		// Only sync (delete/update) templates created and uploaded to objectstore by deployables Reconcile
		// meaning AnnotationHosting must be set in their template
		tplannotations := objtpl.GetAnnotations()
		if tplannotations == nil {
			continue
		}

		if _, ok := tplannotations[appv1alpha1.AnnotationHosting]; !ok {
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
		dpltpl := &unstructured.Unstructured{}

		dpl := chndplmap[name]
		if dpl.Spec.Template == nil {
			klog.Warning("Processing deployable without template:", dpl)
			continue
		}

		err = json.Unmarshal(dpl.Spec.Template.Raw, dpltpl)

		if err != nil {
			klog.Error("Failed to unmashall ", chndesc.Bucket, "/", name, " err:", err)
			continue
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
		dpltplannotations[appv1alpha1.AnnotationHosting] = hosting
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

		if !reflect.DeepEqual(objtpl, dpltpl) {
			dplb, err := yaml.Marshal(dpltpl)
			if err != nil {
				klog.Error("YAML UnMashall ", dpl, " err:", err)
				continue
			}

			dplObj.Content = dplb
			dplObj.Version = dpltplannotations[appv1alpha1.AnnotationDeployableVersion]

			err = chndesc.ObjectStore.Put(chndesc.Bucket, dplObj)
			if err != nil {
				klog.Error("Failed to Put", chndesc.Bucket, "/", name, " err:", err)
			}
		}
	}

	return nil
}
