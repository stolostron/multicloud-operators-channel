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

package helmreposynchronizer

import (
	"context"
	"encoding/json"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/pkg/errors"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/multicloudapps/v1"
	"github.com/open-cluster-management/multicloud-operators-channel/pkg/utils"
	dplv1 "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis/multicloudapps/v1"
	deputils "github.com/open-cluster-management/multicloud-operators-deployable/pkg/utils"
)

// ChannelSynchronizer syncs objectbucket channels with helmrepo
type ChannelSynchronizer struct {
	Scheme       *runtime.Scheme
	kubeClient   client.Client
	Signal       <-chan struct{}
	SyncInterval int
	ChannelMap   map[types.NamespacedName]*chv1.Channel
}

// CreateSynchronizer - creates an instance of ChannelSynchronizer
func CreateHelmrepoSynchronizer(config *rest.Config, scheme *runtime.Scheme, syncInterval int) (*ChannelSynchronizer, error) {
	client, err := client.New(config, client.Options{})
	if err != nil {
		klog.Error("Failed to initialize client for synchronizer. err: ", err)
		return nil, err
	}

	s := &ChannelSynchronizer{
		Scheme:       scheme,
		kubeClient:   client,
		ChannelMap:   make(map[types.NamespacedName]*chv1.Channel),
		SyncInterval: syncInterval,
	}

	return s, nil
}

// Start - starts the sync process
func (sync *ChannelSynchronizer) Start(s <-chan struct{}) error {
	if klog.V(deputils.QuiteLogLel) {
		fnName := deputils.GetFnName()
		klog.Infof("Entering: %v()", fnName)

		defer klog.Infof("Exiting: %v()", fnName)
	}

	sync.Signal = s

	go wait.Until(func() {
		klog.Info("Housekeeping loop ...")
		sync.syncChannelsWithHelmRepo()
	}, time.Duration(sync.SyncInterval)*time.Second, sync.Signal)

	<-s

	return nil
}

// Sync cluster namespace / objectbucket channels with object store

func (sync *ChannelSynchronizer) syncChannelsWithHelmRepo() {
	if klog.V(deputils.QuiteLogLel) {
		fnName := deputils.GetFnName()
		klog.Infof("Entering: %v()", fnName)

		defer klog.Infof("Exiting: %v()", fnName)
	}

	for _, ch := range sync.ChannelMap {
		klog.V(5).Info("synching channel ", ch.Name)
		sync.syncChannel(ch)
	}
}

func (sync *ChannelSynchronizer) syncChannel(chn *chv1.Channel) {
	if chn == nil {
		return
	}

	// retrieve helm chart list from helm repo
	idx, err := utils.GetHelmRepoIndex(chn.Spec.Pathname)
	if err != nil {
		klog.Errorf("Error getting index for channel %v/%v err: %v ", chn.Namespace, chn.Name, errors.Cause(err).Error())
		return
	}

	idx.SortEntries()

	// chartname, chartversion, exists
	generalmap := make(map[string]map[string]bool)
	majorversion := make(map[string]string)

	for k, cv := range idx.Entries {
		klog.V(10).Info("Key: ", k)

		chartmap := make(map[string]bool)

		for _, chart := range cv {
			klog.V(10).Info("Chart:", chart.Name, " Version:", chart.Version)

			chartmap[chart.Version] = false
		}

		generalmap[k] = chartmap
		majorversion[k] = cv[0].Version
	}

	// retrieve deployable list in current channel namespace
	listopt := &client.ListOptions{Namespace: chn.Namespace}
	dpllist := &dplv1.DeployableList{}

	err = sync.kubeClient.List(context.TODO(), dpllist, listopt)
	if err != nil {
		klog.Info("Error in listing deployables in channel namespace: ", chn.Namespace)
		return
	}

	for _, dpl := range dpllist.Items {
		sync.processDeployable(chn, dpl, generalmap)
	}

	for k, charts := range generalmap {
		mv := majorversion[k]
		obj := &unstructured.Unstructured{}
		obj.SetKind(utils.HelmCRKind)
		obj.SetAPIVersion(utils.HelmCRAPIVersion)
		obj.SetName(k)

		specMap := make(map[string]string)
		specMap[utils.HelmCRChartName] = k
		specMap[utils.HelmCRVersion] = mv
		specMap[utils.HelmCRRepoURL] = chn.Spec.Pathname

		obj.Object["spec"] = specMap
		klog.V(10).Info("Object: ", obj.Object)

		if synced := charts[mv]; !synced {
			dpl := &dplv1.Deployable{}
			dpl.Name = chn.GetName() + "-" + obj.GetName() + "-" + mv
			dpl.Namespace = chn.GetNamespace()

			err = controllerutil.SetControllerReference(chn, dpl, sync.Scheme)
			if err != nil {
				klog.Info("Failed to set controller reference err:", err)
				break
			}

			dplanno := make(map[string]string)
			dplanno[dplv1.AnnotationExternalSource] = k
			dplanno[dplv1.AnnotationLocal] = "false"
			dplanno[dplv1.AnnotationDeployableVersion] = mv

			dpl.SetAnnotations(dplanno)
			dpl.Spec.Template = &runtime.RawExtension{}
			dpl.Spec.Template.Raw, err = json.Marshal(obj)

			if err != nil {
				klog.Info("Failed to marshal helm cr to template, err:", err)
				break
			}

			err = sync.kubeClient.Create(context.TODO(), dpl)

			if err != nil {
				klog.Info("Failed to create helmcr deployable, err:", err)
			}

			klog.Info("creating dpl ", k)
		}
	}
}

func (sync *ChannelSynchronizer) processDeployable(chn *chv1.Channel,
	dpl dplv1.Deployable, generalmap map[string]map[string]bool) {
	klog.V(10).Info("synching dpl ", dpl.Name)

	obj := &unstructured.Unstructured{}
	err := json.Unmarshal(dpl.Spec.Template.Raw, obj)

	if err != nil {
		klog.Warning("Processing local deployable with error template:", dpl, err)
		return
	}

	if obj.GetKind() != utils.HelmCRKind || obj.GetAPIVersion() != utils.HelmCRAPIVersion {
		klog.Info("Skipping non helm chart deployable:", obj.GetKind(), ".", obj.GetAPIVersion())
		return
	}

	specMap := obj.Object["spec"].(map[string]interface{})
	cname := specMap[utils.HelmCRChartName].(string)
	cver := specMap[utils.HelmCRVersion].(string)

	keep := false
	chmap := generalmap[cname]

	if cname != "" || cver != "" {
		if chmap != nil {
			if _, ok := chmap[cver]; ok {
				keep = true
			}
		}
	}

	if !keep {
		err = sync.kubeClient.Delete(context.TODO(), &dpl)
		if err != nil {
			klog.Error("Failed to delete deployable in helm repo channel:", dpl.Name, " to ", chn.Spec.Pathname)
		}
	} else {
		chmap[cver] = true
		crepo := specMap[utils.HelmCRRepoURL].(string)
		if crepo != chn.Spec.Pathname {
			specMap[utils.HelmCRRepoURL] = chn.Spec.Pathname
			err = sync.kubeClient.Update(context.TODO(), &dpl)
			if err != nil {
				klog.Error("Failed to update deployable in helm repo channel:", dpl.Name, " to ", chn.Spec.Pathname)
			}
		}
	}
}
