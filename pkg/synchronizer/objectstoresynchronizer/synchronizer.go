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

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"
	"github.com/open-cluster-management/multicloud-operators-channel/pkg/utils"
	dplv1 "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis/apps/v1"
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
		if err := sync.syncChannelsWithObjStore(); err != nil {
			klog.Errorf("failed to run object store synchronizer err: %+v ", err)
			return
		}
	}, time.Duration(sync.SyncInterval)*time.Second, sync.Signal)

	<-sync.Signal

	return nil
}

func (sync *ChannelSynchronizer) syncChannelsWithObjStore() error {
	chlist := &chv1.ChannelList{}
	if err := sync.kubeClient.List(context.TODO(), chlist, &client.ListOptions{}); err != nil {
		return errors.Wrap(err, "sync failed to list all channels")
	}

	if len(chlist.Items) == 0 {
		return nil
	}

	for _, ch := range chlist.Items {
		// Syncying objectbucket channel types only
		if !strings.EqualFold(string(ch.Spec.Type), chv1.ChannelTypeObjectBucket) {
			continue
		}

		if err := sync.alginClusterResourceWithHost(&ch); err != nil {
			klog.Errorf("failed to sync channel %v, err: %+v", ch.GetName(), err)
		}
	}

	return nil
}

func (sync *ChannelSynchronizer) alginClusterResourceWithHost(chn *chv1.Channel) error {
	// injecting objectstore to ease up tests
	if err := sync.ChannelDescriptor.ConnectWithResourceHost(chn, sync.kubeClient, sync.ObjectStore); err != nil {
		return errors.Wrap(err,
			fmt.Sprintf("failed to connect channel %v to it's resources host %v",
				chn.Name, sync.ChannelDescriptor.GetBucketNameByChannel(chn.GetName())))
	}

	//chndesc bundles channel and object bucket info and an objectstore interface
	chndesc, ok := sync.ChannelDescriptor.Get(chn.Name)
	if !ok {
		return errors.New(fmt.Sprintf("failed to get channel description for %v", chn.Name))
	}

	hostResMap, err := getResourceMapFromHost(chndesc.ObjectStore, chndesc.Bucket)
	if err != nil {
		return errors.Wrap(err, "failed to get resources map from host")
	}

	// processed item from hostResMap will be deleted
	if err := sync.deleteOrUpdateDeployableBasedOnHostResMap(chn, hostResMap); err != nil {
		return errors.Wrap(err, "failed to deleteOrUpdateDeployableBasedOnHostMap")
	}

	sync.addNewResourceFromHostResMap(chn, hostResMap)

	return nil
}

func getResourceMapFromHost(host utils.ObjectStore, bucketName string) (map[string]*unstructured.Unstructured, error) {
	objnames, err := host.List(bucketName)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to list objects in bucket %v", bucketName))
	}

	resMap := make(map[string]*unstructured.Unstructured)
	for _, name := range objnames {
		objb, err := host.Get(bucketName, name)
		if err != nil {
			klog.Errorf("failed to get object %v/%v err: %v", bucketName, name, err)
			continue
		}

		objtpl := &unstructured.Unstructured{}

		if err := yaml.Unmarshal(objb.Content, objtpl); err != nil {
			klog.Errorf("failed to unmashall %v/%v err: %v", bucketName, name, err)
			continue
		}

		resMap[name] = objtpl
	}

	return resMap, nil
}

func (sync *ChannelSynchronizer) deleteOrUpdateDeployableBasedOnHostResMap(chn *chv1.Channel, hostResMap map[string]*unstructured.Unstructured) error {
	if chn == nil {
		return errors.New("failed to update deployable resources, nil of channel")
	}

	dpllist := &dplv1.DeployableList{}
	if err := sync.kubeClient.List(context.TODO(), dpllist, &client.ListOptions{Namespace: chn.GetNamespace()}); err != nil {
		return errors.Wrap(err, "failed to list all deployables")
	}

	for _, dpl := range dpllist.Items {
		if err := sync.deleteOrUpdateDeployable(hostResMap, dpl, chn); err != nil {
			klog.Error(errors.Cause(err).Error())
		}
	}

	return nil
}

func (sync *ChannelSynchronizer) addNewResourceFromHostResMap(chn *chv1.Channel, hostResMap map[string]*unstructured.Unstructured) {
	if len(hostResMap) == 0 {
		return
	}
	// Add new resources to channel namespace
	for tplname, tpl := range hostResMap {
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
		var err error
		dpl.Name = tplname
		dpl.Namespace = chn.Namespace
		dpl.Spec.Template = &runtime.RawExtension{}
		dpl.Spec.Template.Raw, err = json.Marshal(tpl)

		if err != nil {
			klog.Error("failed to mashall template ", tpl)
			continue
		}

		klog.Infof("creating deployable %v in channel %v", tplname, chn.GetName())

		if err := sync.kubeClient.Create(context.TODO(), dpl); err != nil {
			klog.Errorf("failed to create deployable %v from hostResMap with error: %v ", err, dpl)
		}
	}

	return
}

func (sync *ChannelSynchronizer) deleteOrUpdateDeployable(
	hostResMap map[string]*unstructured.Unstructured, dpl dplv1.Deployable,
	chn *chv1.Channel) error {
	dpltpl, err := getUnstructuredTemplateFromDeployable(&dpl)
	if err != nil {
		klog.Errorf("failed to get valid deployable template err %+v", err)
		delete(hostResMap, dpl.Name)
		return nil
	}

	// Delete deployables that don't exist in the bucket anymore
	if _, ok := hostResMap[dpl.Name]; !ok {
		klog.Info("Sync - Deleting deployable ", dpl.Namespace, "/", dpl.Name, " from channel ", chn.Name)

		if err := sync.kubeClient.Delete(context.TODO(), &dpl); err != nil {
			return errors.Wrap(err, fmt.Sprintf("sync failed to delete deployable %v", dpl.Name))
		}

		return nil
	}

	// Update deployables if they are updated in the bucket
	// Ignore deployable AnnotationExternalSource when comparing with template
	tpl := hostResMap[dpl.Name]
	tplannotations := tpl.GetAnnotations()

	if tplannotations == nil {
		tplannotations = make(map[string]string)
	}

	tplannotations[dplv1.AnnotationExternalSource] = chn.Spec.Pathname
	tpl.SetAnnotations(tplannotations)

	if !reflect.DeepEqual(tpl, dpltpl) {
		dpl.Spec.Template.Raw, err = json.Marshal(tpl)
		if err != nil {
			delete(hostResMap, dpl.Name)
			return errors.Wrap(err, fmt.Sprintf("failed to mashall template %v", tpl))
		}

		klog.Info("Sync - Updating existing deployable ", dpl.Name, " in channel ", chn.Name)

		if err := sync.kubeClient.Update(context.TODO(), &dpl); err != nil {
			return errors.Wrap(err, fmt.Sprintf("failed to update deployable %v", dpl.Name))
		}
	}

	delete(hostResMap, dpl.Name)

	return nil
}

// getUnstructuredTemplateFromDeployable tests if deployable has a valid teamplate and if it's created
// by a synchronizer. If true, return the deployable template otherwise, error will be given to the caller
func getUnstructuredTemplateFromDeployable(dpl *dplv1.Deployable) (*unstructured.Unstructured, error) {
	dpltpl := &unstructured.Unstructured{}

	if dpl.Spec.Template == nil {
		return nil, errors.New(fmt.Sprintf("skipped deployable %v due to empty template", dpl.GetName()))
	}

	if err := json.Unmarshal(dpl.Spec.Template.Raw, dpltpl); err != nil {
		return nil, errors.Errorf("failed to unmarshal %v template, err: %v", dpl.GetName(), err)
	}

	// Only sync (delete/update) deployables created by this synchronizer
	// meaning AnnotationExternalSource must be set in their template
	dpltplannotations := dpltpl.GetAnnotations()
	if dpltplannotations == nil {
		return nil, errors.Errorf("skipped deployable %v since it is not created by this synchronizer", dpl.GetName())
	}

	if _, ok := dpltplannotations[dplv1.AnnotationExternalSource]; !ok {
		return nil, errors.Errorf("skipped deployable %v since it is not created by this synchronizer", dpl.GetName())
	}

	return dpltpl, nil
}
