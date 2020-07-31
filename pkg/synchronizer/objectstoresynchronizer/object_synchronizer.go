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
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"
	"github.com/open-cluster-management/multicloud-operators-channel/pkg/utils"
	dplv1 "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis/apps/v1"
)

const (
	syncrhonizerName = "objectbucket"
)

var (
	syncLog = logf.Log.WithName("sync" + syncrhonizerName)
)

// ChannelSynchronizer syncs objectbucket channels with ObjectStore
type ChannelSynchronizer struct {
	EnableForClusterNamespace bool
	EnableForChannel          bool
	kubeClient                client.Client
	ObjectStore               utils.ObjectStore
	Signal                    <-chan struct{}
	SyncInterval              int

	//this is created during manager start up time and shared with reconcile.
	// ChannelDescriptor, holds a map of channels unit
	ChannelDescriptor *utils.ChannelDescriptor
}

// CreateSynchronizer - creates an instance of ChannelSynchronizer
func CreateObjectStoreSynchronizer(config *rest.Config, chdesc *utils.ChannelDescriptor, syncInterval int) (*ChannelSynchronizer, error) {
	client, err := client.New(config, client.Options{})
	if err != nil {
		syncLog.Error(err, "Failed to initialize client for synchronizer.")
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
	sync.Signal = s

	go wait.Until(func() {
		if err := sync.syncChannelsWithObjStore(); err != nil {
			syncLog.Error(err, "failed to run object store synchronizer")
			return
		}
		syncLog.Info("housekeeping object store synchronizer")
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
		ch := ch
		// Syncying objectbucket channel types only
		if !strings.EqualFold(string(ch.Spec.Type), chv1.ChannelTypeObjectBucket) {
			continue
		}

		if err := sync.alginClusterResourceWithHost(&ch); err != nil {
			syncLog.Error(err, fmt.Sprintf("failed to sync channel %v", ch.GetName()))
		}
	}

	return nil
}

func (sync *ChannelSynchronizer) alginClusterResourceWithHost(chn *chv1.Channel) error {
	// injecting objectstore to ease up tests
	if err := sync.ChannelDescriptor.ConnectWithResourceHost(chn, sync.kubeClient, syncLog, sync.ObjectStore); err != nil {
		return errors.Wrap(err,
			fmt.Sprintf("failed to connect channel %v to it's resources host %v",
				chn.Name, sync.ChannelDescriptor.GetBucketNameByChannel(chn.GetName())))
	}

	//chndesc bundles channel and object bucket info and an objectstore interface
	chUnit, ok := sync.ChannelDescriptor.Get(chn.Name)
	if !ok {
		return errors.New(fmt.Sprintf("failed to get channel description for %v", chn.Name))
	}

	hostResMap, err := getResourceMapFromHost(chUnit.ObjectStore, chUnit.Bucket)
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
			syncLog.Error(err, fmt.Sprintf("failed to get object %v/%v", bucketName, name))
			continue
		}

		objtpl := &unstructured.Unstructured{}

		if err := yaml.Unmarshal(objb.Content, objtpl); err != nil {
			syncLog.Error(err, fmt.Sprintf("failed to unmashall %v/%v", bucketName, name))
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
			syncLog.Error(err, "failed to update deployable")
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
		}

		if v, ok := tplannotations[dplv1.AnnotationHosting]; ok {
			vkey := utils.SplitStringToTypes(v)
			tmp := &dplv1.Deployable{}

			//meaning the deployable exist in the channel's namespace
			if err := sync.kubeClient.Get(context.TODO(), vkey, tmp); err == nil {
				continue
			}
		}
		// Set AnnotationExternalSource

		tplannotations[dplv1.AnnotationExternalSource] = chn.Spec.Pathname
		tplannotations[dplv1.AnnotationLocal] = "false"
		tpl.SetAnnotations(tplannotations)

		var err error

		dpl := &dplv1.Deployable{}
		dpl.Name = tplname
		dpl.Namespace = chn.Namespace
		dpl.Spec.Template = &runtime.RawExtension{}
		dpl.Spec.Template.Raw, err = json.Marshal(tpl)

		// for hub subscription to get its subscribing resource
		addL := map[string]string{
			chv1.KeyChannel:     chn.GetName(),
			chv1.KeyChannelType: string(chn.Spec.Type),
		}

		utils.AddOrAppendChannelLabel(dpl, addL)

		syncLog.Info(fmt.Sprintf("aaaaa creating deployable %v in channel %v", tplname, chn.GetName()))

		if err != nil {
			syncLog.Error(err, "failed to marshal template ")
			continue
		}

		syncLog.Info(fmt.Sprintf("creating deployable %v in channel %v", tplname, chn.GetName()))

		if err := sync.kubeClient.Create(context.TODO(), dpl); err != nil {
			syncLog.Error(err, fmt.Sprintf("failed to create deployable %v from hostResMap", dpl))
		}
	}
}

func (sync *ChannelSynchronizer) deleteOrUpdateDeployable(
	hostResMap map[string]*unstructured.Unstructured, dpl dplv1.Deployable,
	chn *chv1.Channel) error {
	dpltpl, err := getUnstructuredTemplateFromDeployable(&dpl)
	if err != nil {
		syncLog.Error(err, "failed to get valid deployable template")
		delete(hostResMap, dpl.Name)

		return nil
	}

	// Only sync (delete/update) deployables created by this synchronizer
	// meaning AnnotationExternalSource must be set in their template
	if !isDeployableCreatedBySynchronizer(dpltpl) {
		syncLog.Info(fmt.Sprintf("skip deployable %v/%v, not created by object sychronizer", dpltpl.GetName(), dpltpl.GetNamespace()))
		delete(hostResMap, dpltpl.GetName())

		return nil
	}

	// Delete deployables that don't exist in the bucket anymore
	if _, ok := hostResMap[dpl.Name]; !ok {
		syncLog.Info(fmt.Sprintf("Sync - Deleting deployable %v/%v for channel %v", dpl.Namespace, dpl.Name, chn.Name))

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

		syncLog.Info(fmt.Sprintf("Sync - Updating existing deployable %v in channel %v", dpl.Name, chn.Name))

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

	return dpltpl, nil
}

func isDeployableCreatedBySynchronizer(dpltpl *unstructured.Unstructured) bool {
	dpltplannotations := dpltpl.GetAnnotations()
	if dpltplannotations == nil {
		return false
	}

	if _, ok := dpltplannotations[dplv1.AnnotationExternalSource]; !ok {
		return false
	}

	return true
}
