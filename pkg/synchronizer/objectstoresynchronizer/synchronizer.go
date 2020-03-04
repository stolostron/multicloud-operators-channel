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
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/multicloudapps/v1"
	"github.com/open-cluster-management/multicloud-operators-channel/pkg/utils"
	dplv1 "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis/multicloudapps/v1"
	dplutils "github.com/open-cluster-management/multicloud-operators-deployable/pkg/utils"
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
func CreateObjectStoreSynchronizer(config *rest.Config, chdesc *utils.ChannelDescriptor, syncInterval int) (*ChannelSynchronizer, error) {
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
		_ = sync.syncWithObjectStore()
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
		return err
	}

	return nil
}

func (sync *ChannelSynchronizer) syncChannelsWithObjStore() error {
	chlist := &chv1.ChannelList{}
	err := sync.kubeClient.List(context.TODO(), chlist, &client.ListOptions{})

	if err != nil {
		klog.Error(err, "Sync - Failed to list all channels ")
		return nil
	}

	for _, ch := range chlist.Items {
		// Syncying objectbucket channel types only
		if !strings.EqualFold(string(ch.Spec.Type), chv1.ChannelTypeObjectBucket) {
			continue
		}

		if err := sync.syncChannel(&ch); err != nil {
			klog.Error(errors.Cause(err).Error())
		}
	}

	return nil
}

func (sync *ChannelSynchronizer) syncChannel(chn *chv1.Channel) error {
	if err := sync.ChannelDescriptor.ValidateChannel(chn, sync.kubeClient); err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to validate channel %v", chn.Name))
	}

	chndesc, ok := sync.ChannelDescriptor.Get(chn.Name)
	if !ok {
		return errors.New(fmt.Sprintf("failed to get channel description for %v", chn.Name))
	}

	objnames, err := chndesc.ObjectStore.List(chndesc.Bucket)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to list objects in bucket %v", chndesc.Bucket))
	}

	tplmap := make(map[string]*unstructured.Unstructured)

	for _, name := range objnames {
		objb, err := chndesc.ObjectStore.Get(chndesc.Bucket, name)
		if err != nil {
			klog.Error("Sync - Failed to get object ", chndesc.Bucket, "/", name, " err:", err)
			continue
		}

		objtpl := &unstructured.Unstructured{}
		err = yaml.Unmarshal(objb.Content, objtpl)

		if err != nil {
			klog.Error("Sync - Failed to unmashall ", chndesc.Bucket, "/", name, " err:", err)
			continue
		}

		tplmap[name] = objtpl
	}

	dpllist := &dplv1.DeployableList{}
	if err := sync.kubeClient.List(context.TODO(), dpllist, &client.ListOptions{Namespace: chn.GetNamespace()}); err != nil {
		return errors.Wrap(err, "failed to list all deployables")
	}

	for _, dpl := range dpllist.Items {
		if err := sync.updateSynchronizerWithDeployable(tplmap, dpl, chn); err != nil {
			klog.Error(errors.Cause(err).Error())
		}
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
		} else if _, ok := tplannotations[dplv1.AnnotationHosting]; ok {
			continue
		}
		// Set AnnotationExternalSource
		tplannotations[dplv1.AnnotationExternalSource] = chn.Spec.Pathname
		tplannotations[dplv1.AnnotationLocal] = "false"
		tpl.SetAnnotations(tplannotations)

		dpl := &dplv1.Deployable{}
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

func (sync *ChannelSynchronizer) updateSynchronizerWithDeployable(
	tplmap map[string]*unstructured.Unstructured, dpl dplv1.Deployable,
	chn *chv1.Channel) error {
	dpltpl, err := getUnstructuredTemplateFromDeployable(&dpl)
	if err != nil {
		delete(tplmap, dpl.Name)
		return nil
	}

	// Delete deployables that don't exist in the bucket anymore
	if _, ok := tplmap[dpl.Name]; !ok {
		klog.Info("Sync - Deleting deployable ", dpl.Namespace, "/", dpl.Name, " from channel ", chn.Name)

		if err := sync.kubeClient.Delete(context.TODO(), &dpl); err != nil {
			return errors.Wrap(err, fmt.Sprintf("sync failed to delete deployable %v", dpl.Name))
		}
	}

	// Update deployables if they are updated in the bucket
	// Ignore deployable AnnotationExternalSource when comparing with template
	tpl := tplmap[dpl.Name]
	tplannotations := tpl.GetAnnotations()

	if tplannotations == nil {
		tplannotations = make(map[string]string)
	}

	tplannotations[dplv1.AnnotationExternalSource] = chn.Spec.Pathname
	tpl.SetAnnotations(tplannotations)

	if !reflect.DeepEqual(tpl, dpltpl) {
		dpl.Spec.Template.Raw, err = json.Marshal(tpl)
		if err != nil {
			delete(tplmap, dpl.Name)
			return errors.Wrap(err, fmt.Sprintf("failed to mashall template %v", tpl))
		}

		klog.Info("Sync - Updating existing deployable ", dpl.Name, " in channel ", chn.Name)

		if err := sync.kubeClient.Update(context.TODO(), &dpl); err != nil {
			return errors.Wrap(err, fmt.Sprintf("failed to update deployable %v", dpl.Name))
		}
	}

	delete(tplmap, dpl.Name)

	return nil
}

// getUnstructuredTemplateFromDeployable return error if needed
func getUnstructuredTemplateFromDeployable(dpl *dplv1.Deployable) (*unstructured.Unstructured, error) {
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

	if _, ok := dpltplannotations[dplv1.AnnotationExternalSource]; !ok {
		return nil, errors.New("Deployable is not created by this synchronizer:" + dpl.GetName())
	}

	return dpltpl, nil
}
