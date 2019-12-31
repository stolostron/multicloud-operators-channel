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

package objectstoresynchronizer

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/ghodss/yaml"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"

	chnv1alpha1 "github.com/IBM/multicloud-operators-channel/pkg/apis/app/v1alpha1"
	"github.com/IBM/multicloud-operators-channel/pkg/utils"
	dplv1alpha1 "github.com/IBM/multicloud-operators-deployable/pkg/apis/app/v1alpha1"
	dplutils "github.com/IBM/multicloud-operators-deployable/pkg/utils"
)

// ChannelSynchronizer syncs objectbucket channels with ObjectStore
type ChannelSynchronizer struct {
	EnableForClusterNamespace bool
	EnableForChannel          bool
	kubeClient                client.Client
	ObjectStore               utils.ObjectStore
	Signal                    <-chan struct{}
	SyncInterval              int
	ChannelDescriptor         *utils.ChannelDescriptor
}

// CreateSynchronizer - creates an instance of ChannelSynchronizer
func CreateSynchronizer(config *rest.Config, chdesc *utils.ChannelDescriptor, syncInterval int) (*ChannelSynchronizer, error) {
	client, err := client.New(config, client.Options{})
	if err != nil {
		klog.Error("Failed to initialize client for synchronizer. err: ", err)
		return nil, err
	}

	s := &ChannelSynchronizer{
		kubeClient:        client,
		ChannelDescriptor: chdesc,
		SyncInterval:      syncInterval,
	}

	return s, nil
}

// Start - starts the sync process
func (sync *ChannelSynchronizer) Start(s <-chan struct{}) error {
	if klog.V(dplutils.QuiteLogLel) {
		fnName := dplutils.GetFnName()
		klog.Infof("Entering: %v()", fnName)

		defer klog.Infof("Exiting: %v()", fnName)
	}

	sync.Signal = s

	go wait.Until(func() {
		sync.syncWithObjectStore()
	}, time.Duration(sync.SyncInterval)*time.Second, sync.Signal)

	<-sync.Signal

	return nil
}

// Sync cluster namespace / objectbucket channels with object store
func (sync *ChannelSynchronizer) syncWithObjectStore() error {
	if klog.V(dplutils.QuiteLogLel) {
		fnName := dplutils.GetFnName()
		klog.Infof("Entering: %v()", fnName)

		defer klog.Infof("Exiting: %v()", fnName)
	}

	err := sync.syncChannelsWithObjStore()
	if err != nil {
		klog.Error(err, "Sync - Failed to sync Channels With ObjectStore")
	}

	return nil
}

func (sync *ChannelSynchronizer) syncChannelsWithObjStore() error {
	chlist := &chnv1alpha1.ChannelList{}
	err := sync.kubeClient.List(context.TODO(), chlist, &client.ListOptions{})

	if err != nil {
		klog.Error(err, "Sync - Failed to list all channels ")
		return nil
	}

	for _, ch := range chlist.Items {
		// Syncying objectbucket channel types only
		if strings.ToLower(string(ch.Spec.Type)) != chnv1alpha1.ChannelTypeObjectBucket {
			continue
		}

		sync.syncChannel(&ch)
	}

	return nil
}

func (sync *ChannelSynchronizer) syncChannel(chn *chnv1alpha1.Channel) error {
	err := sync.ChannelDescriptor.ValidateChannel(chn, sync.kubeClient)
	if err != nil {
		klog.Info("Sync - Failed to validate channel ", chn.Name, " err:", err)
		return nil
	}

	chndesc, ok := sync.ChannelDescriptor.Get(chn.Name)
	if !ok {
		klog.Info("Sync - Failed to get channel description for ", chn.Name)
		return nil
	}

	objnames, err := chndesc.ObjectStore.List(chndesc.Bucket)
	if err != nil {
		klog.Info("Sync - Failed to list objects in bucket ", chndesc.Bucket, " channel ", chndesc.Channel.Name, " err:", err)
		return nil
	}

	tplmap := make(map[string]*unstructured.Unstructured)

	for _, name := range objnames {
		objb, err := chndesc.ObjectStore.Get(chndesc.Bucket, name)
		if err != nil {
			klog.Error("Sync - Failed to get object ", chndesc.Bucket, "/", name, " err:", err)
			return nil
		}

		objtpl := &unstructured.Unstructured{}
		err = yaml.Unmarshal(objb.Content, objtpl)

		if err != nil {
			klog.Error("Sync - Failed to unmashall ", chndesc.Bucket, "/", name, " err:", err)
			continue
		}

		tplmap[name] = objtpl
	}

	dpllist := &dplv1alpha1.DeployableList{}
	err = sync.kubeClient.List(context.TODO(), dpllist, &client.ListOptions{Namespace: chn.GetNamespace()})

	if err != nil {
		klog.Error("Sync - Failed to list all deployable", " err:", err)
		return nil
	}

	for _, dpl := range dpllist.Items {
		dpltpl, err := getUnstructuredTemplateFromDeployable(&dpl)
		if err != nil {
			delete(tplmap, dpl.Name)
			continue
		}

		// Delete deployables that don't exist in the bucket anymore
		if _, ok := tplmap[dpl.Name]; !ok {
			klog.Info("Sync - Deleting deployable ", dpl.Namespace, "/", dpl.Name, " from channel ", chn.Name)
			err = sync.kubeClient.Delete(context.TODO(), &dpl)

			if err != nil {
				klog.Error("Sync - Failed to delete deployable with error: ", err, " with ", dpl.Name)
			}

			continue
		}

		// Update deployables if they are updated in the bucket
		// Ignore deployable AnnotationExternalSource when comparing with template
		tpl := tplmap[dpl.Name]
		tplannotations := tpl.GetAnnotations()

		if tplannotations == nil {
			tplannotations = make(map[string]string)
		}

		tplannotations[dplv1alpha1.AnnotationExternalSource] = chn.Spec.PathName
		tpl.SetAnnotations(tplannotations)

		if !reflect.DeepEqual(tpl, dpltpl) {
			dpl.Spec.Template.Raw, err = json.Marshal(tpl)
			if err != nil {
				klog.Info("Sync - Error in mashalling template ", tpl)
				delete(tplmap, dpl.Name)

				continue
			}

			klog.Info("Sync - Updating existing deployable ", dpl.Name, " in channel ", chn.Name)
			err = sync.kubeClient.Update(context.TODO(), &dpl)

			if err != nil {
				klog.Error("Sync - Failed to update deployable with error: ", err, " with ", dpl.Name)
			}
		}

		delete(tplmap, dpl.Name)
	}

	// Add new resources to channel namespace
	for tplname, tpl := range tplmap {
		if tpl == nil {
			continue
		}

		// Only add resources that are not created by deployable-objectstore controller
		// meaning AnnotationHosting is not set in their template
		tplannotations := tpl.GetAnnotations()
		if tplannotations == nil {
			tplannotations = make(map[string]string)
		} else if _, ok := tplannotations[dplv1alpha1.AnnotationHosting]; ok {
			continue
		}
		// Set AnnotationExternalSource
		tplannotations[dplv1alpha1.AnnotationExternalSource] = chn.Spec.PathName
		tplannotations[dplv1alpha1.AnnotationLocal] = "false"
		tpl.SetAnnotations(tplannotations)

		dpl := &dplv1alpha1.Deployable{}
		dpl.Name = tplname
		dpl.Namespace = chn.Namespace
		dpl.Spec.Template = &runtime.RawExtension{}
		dpl.Spec.Template.Raw, err = json.Marshal(tpl)

		if err != nil {
			klog.Info("Sync - Error in mashalling template ", tpl)
			continue
		}

		klog.Info("Sync - Creating deployable ", tplname, " in channel ", chn.GetName())

		err = sync.kubeClient.Create(context.TODO(), dpl)

		if err != nil {
			klog.Error("Sync - Failed to create deployable with error: ", err, " with ", dpl)
		}
	}

	return nil
}

// getUnstructuredTemplateFromDeployable return error if needed
func getUnstructuredTemplateFromDeployable(dpl *dplv1alpha1.Deployable) (*unstructured.Unstructured, error) {
	dpltpl := &unstructured.Unstructured{}

	if dpl.Spec.Template == nil {
		return nil, errors.New("Processing deployable without template:" + dpl.GetName())
	}

	err := json.Unmarshal(dpl.Spec.Template.Raw, dpltpl)

	if err != nil {
		klog.Error("Sync - Failed to unmarshal template with error: ", err, " with ", string(dpl.Spec.Template.Raw))
		return nil, err
	}

	// Only sync (delete/update) deployables created by this synchronizer
	// meaning AnnotationExternalSource must be set in their template
	dpltplannotations := dpltpl.GetAnnotations()
	if dpltplannotations == nil {
		return nil, errors.New("Deployable is not created by this synchronizer:" + dpl.GetName())
	}

	if _, ok := dpltplannotations[dplv1alpha1.AnnotationExternalSource]; !ok {
		return nil, errors.New("Deployable is not created by this synchronizer:" + dpl.GetName())
	}

	return dpltpl, nil
}
