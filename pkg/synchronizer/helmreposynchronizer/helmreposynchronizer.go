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
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"
	"github.com/open-cluster-management/multicloud-operators-channel/pkg/utils"
	dplv1 "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis/apps/v1"
	deputils "github.com/open-cluster-management/multicloud-operators-deployable/pkg/utils"
)

const syncrhonizerName = "helmrepo"

var (
	logf = log.Log.WithName(syncrhonizerName)
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
		logf.Error(err, "Failed to initialize client for synchronizer.")
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
	logf.Info(fmt.Sprintf("start %v sync loop", syncrhonizerName))
	defer logf.Info(fmt.Sprintf("exit %v sync loop", syncrhonizerName))

	sync.Signal = s

	go wait.Until(func() {
		logf.Info("helmrepo housekeeping loop ...")
		sync.syncChannelsWithHelmRepo()
	}, time.Duration(sync.SyncInterval)*time.Second, sync.Signal)

	<-s

	return nil
}

// Sync cluster namespace / objectbucket channels with object store

func (sync *ChannelSynchronizer) syncChannelsWithHelmRepo() {
	for _, ch := range sync.ChannelMap {
		logf.V(5).Info(fmt.Sprintf("synching channel %v", ch.Name))
		sync.syncChannel(ch, utils.GetChartIndex)
	}
}

func (sync *ChannelSynchronizer) syncChannel(chn *chv1.Channel, localIdxFunc utils.LoadIndexPageFunc) {
	if chn == nil {
		return
	}

	chnRefCfgMap := &corev1.ConfigMap{}

	if chn.Spec.ConfigMapRef != nil {
		if chn.Spec.ConfigMapRef.Namespace == "" {
			chn.Spec.ConfigMapRef.Namespace = chn.GetNamespace()
		}

		chnRefCfgMapKey := types.NamespacedName{Name: chn.Spec.ConfigMapRef.Name, Namespace: chn.Spec.ConfigMapRef.Namespace}
		if err := sync.kubeClient.Get(context.TODO(), chnRefCfgMapKey, chnRefCfgMap); err != nil {
			logf.Error(err, "failed to Get channel's referred configmap")
		}
	}

	chnRefSrt := &corev1.Secret{}
	if chn.Spec.SecretRef != nil {
		if chn.Spec.SecretRef.Namespace == "" {
			chn.Spec.SecretRef.Namespace = chn.GetNamespace()
		}

		chnRefSrtKey := types.NamespacedName{Name: chn.Spec.SecretRef.Name, Namespace: chn.Spec.SecretRef.Namespace}
		if err := sync.kubeClient.Get(context.TODO(), chnRefSrtKey, chnRefSrt); err != nil {
			logf.Error(err, "failed to Get channel's referred configmap")
		}
	}

	// retrieve helm chart list from helm repo
	idx, err := utils.GetHelmRepoIndex(chn.Spec.Pathname, chn.Spec.InsecureSkipVerify, chnRefSrt, chnRefCfgMap, localIdxFunc, logf)
	if err != nil {
		logf.Error(err, fmt.Sprintf("error getting index for channel %v/%v", chn.Namespace, chn.Name))
		return
	}

	idx.SortEntries()

	// chartname, chartversion, exists
	generalmap := make(map[string]map[string]bool)
	majorversion := make(map[string]string)

	for k, cv := range idx.Entries {
		logf.V(5).Info(fmt.Sprintf("Key: %v", k))

		chartmap := make(map[string]bool)

		for _, chart := range cv {
			logf.V(5).Info(fmt.Sprintf("Chart: %v Version: %v", chart.Name, chart.Version))

			chartmap[chart.Version] = false
		}

		generalmap[k] = chartmap
		majorversion[k] = cv[0].Version
	}

	// retrieve deployable list in current channel namespace
	listopt := &client.ListOptions{Namespace: chn.Namespace}

	// Handle deployables from multiple channels in the same namespace
	chLabel := make(map[string]string)
	chLabel[chv1.KeyChannel] = chn.Name
	chLabel[chv1.KeyChannelType] = string(chn.Spec.Type)
	labelSelector := &metav1.LabelSelector{
		MatchLabels: chLabel,
	}

	clSelector, err := deputils.ConvertLabels(labelSelector)
	if err != nil {
		logf.Error(err, "failed to set label selector.")
		return
	}

	listopt.LabelSelector = clSelector

	dpllist := &dplv1.DeployableList{}

	err = sync.kubeClient.List(context.TODO(), dpllist, listopt)
	if err != nil {
		logf.Error(err, fmt.Sprintf("error in listing deployables in channel namespace: %v ", chn.Namespace))
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
		logf.V(10).Info(fmt.Sprintf("Object: %v", obj.Object))

		if synced := charts[mv]; !synced {
			dpl := &dplv1.Deployable{}
			dpl.Name = chn.GetName() + "-" + obj.GetName() + "-" + mv
			dpl.Namespace = chn.GetNamespace()

			err = controllerutil.SetControllerReference(chn, dpl, sync.Scheme)
			if err != nil {
				logf.Error(err, "failed to set controller reference ")
				break
			}

			dplanno := make(map[string]string)
			dplanno[dplv1.AnnotationExternalSource] = k
			dplanno[dplv1.AnnotationLocal] = "false"
			dplanno[dplv1.AnnotationDeployableVersion] = mv

			dpl.SetAnnotations(dplanno)

			dplLabels := make(map[string]string)
			dplLabels[chv1.KeyChannel] = chn.Name
			dplLabels[chv1.KeyChannelType] = string(chn.Spec.Type)
			dpl.SetLabels(dplLabels)

			dpl.Spec.Template = &runtime.RawExtension{}
			dpl.Spec.Template.Raw, err = json.Marshal(obj)

			if err != nil {
				logf.Error(err, "Failed to marshal helm cr to template")
				break
			}

			err = sync.kubeClient.Create(context.TODO(), dpl)

			if err != nil {
				logf.Error(err, "failed to create helmcr deployable")
			}

			logf.Info(fmt.Sprintf("creating dpl %v", k))
		}
	}
}

func (sync *ChannelSynchronizer) processDeployable(chn *chv1.Channel,
	dpl dplv1.Deployable, generalmap map[string]map[string]bool) {
	logf.V(5).Info(fmt.Sprintf("synching dpl %v", dpl.Name))

	obj := &unstructured.Unstructured{}
	err := json.Unmarshal(dpl.Spec.Template.Raw, obj)

	if err != nil {
		logf.Error(err, fmt.Sprintf("Processing local deployable with error template: %v", dpl))
		return
	}

	if obj.GetKind() != utils.HelmCRKind || obj.GetAPIVersion() != utils.HelmCRAPIVersion {
		logf.Info(fmt.Sprintf("Skipping non helm chart deployable: %v.%v", obj.GetKind(), obj.GetAPIVersion()))
		return
	}

	// Do not delete deployables with Subscription template kind. This causes problems if subscription
	// and channel are in the same namespace.
	if obj.GetKind() == utils.SubscriptionCRKind {
		logf.Info(fmt.Sprintf("Skipping subscription deployable: %v.%v", dpl.Name, obj.GetKind()))
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
			logf.Error(err, fmt.Sprintf("Failed to delete deployable in helm repo channel: %v to %v", dpl.Name, chn.Spec.Pathname))
		}
	} else {
		chmap[cver] = true
		crepo := specMap[utils.HelmCRRepoURL].(string)
		if crepo != chn.Spec.Pathname {
			specMap[utils.HelmCRRepoURL] = chn.Spec.Pathname
			err = sync.kubeClient.Update(context.TODO(), &dpl)
			if err != nil {
				logf.Error(err, fmt.Sprintf("Failed to update deployable in helm repo channel: %v to %v", dpl.Name, chn.Spec.Pathname))
			}
		}
	}
}
