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

package githubsynchronizer

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/helm/pkg/repo"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/multicloudapps/v1"
	"github.com/open-cluster-management/multicloud-operators-channel/pkg/utils"
	dplv1alpha1 "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis/multicloudapps/v1alpha1"
	deputils "github.com/open-cluster-management/multicloud-operators-deployable/pkg/utils"
)

const (
	debugLevel = klog.Level(10)
)

// ChannelSynchronizer syncs github channels with github repository
type ChannelSynchronizer struct {
	Scheme       *runtime.Scheme
	kubeClient   client.Client
	Signal       <-chan struct{}
	SyncInterval int
	ChannelMap   map[types.NamespacedName]*chv1.Channel
}

type helmTemplate struct {
	APIVersion string    `json:"apiVersion,omitempty"`
	Kind       string    `json:"kind,omitempty"`
	Spec       *helmSpec `json:"spec,omitempty"`
}

type helmSpec struct {
	ChartName   string  `json:"chartName,omitempty"`
	ReleaseName string  `json:"releaseName,omitempty"`
	Version     string  `json:"version,omitempty"`
	Source      *source `json:"source,omitempty"`
}

type source struct {
	HelmRepo *sourceURLs `json:"helmRepo,omitempty"`
	GitHub   *sourceURLs `json:"github,omitempty"`
	Type     string      `json:"type,omitempty"`
}

type sourceURLs struct {
	URLs      []string `json:"urls,omitempty"`
	ChartPath string   `json:"chartPath,omitempty"`
}

type kubeResource struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   *kubeResourceMetadata
}

type kubeResourceMetadata struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace"`
}

// CreateGithubSynchronizer - creates an instance of ChannelSynchronizer
func CreateGithubSynchronizer(config *rest.Config, scheme *runtime.Scheme, syncInterval int) (*ChannelSynchronizer, error) {
	client, err := client.New(config, client.Options{})
	if err != nil {
		klog.Error("Failed to initialize client for synchronizer. err: ", err)
		return nil, err
	}

	s := &ChannelSynchronizer{
		Scheme:       scheme,
		kubeClient:   client,
		ChannelMap:   make(map[types.NamespacedName]*chnv1alpha1.Channel),
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
		sync.syncChannelsWithGitRepo()
	}, time.Duration(sync.SyncInterval)*time.Second, sync.Signal)

	<-s

	return nil
}

// Sync cluster namespace / github channels with object store

func (sync *ChannelSynchronizer) syncChannelsWithGitRepo() {
	if klog.V(deputils.QuiteLogLel) {
		fnName := deputils.GetFnName()
		klog.Infof("Entering: %v()", fnName)

		defer klog.Infof("Exiting: %v()", fnName)
	}

	for _, ch := range sync.ChannelMap {
		klog.V(debugLevel).Info("synching channel ", ch.Name)
		sync.syncChannel(ch)
	}
}

func (sync *ChannelSynchronizer) processYamlFile(chn *chv1.Channel, files []os.FileInfo, dir string, newDplList map[string]*dplv1alpha1.Deployable) {
	for _, f := range files {
		// If YAML or YML,
		if f.Mode().IsRegular() {
			if strings.EqualFold(filepath.Ext(f.Name()), ".yml") || strings.EqualFold(filepath.Ext(f.Name()), ".yaml") {
				// check it it is Kubernetes resource
				klog.V(debugLevel).Info("scanning file ", f.Name())
				file, _ := ioutil.ReadFile(filepath.Join(dir, f.Name()))
				t := kubeResource{}

				err := yaml.Unmarshal(file, &t)
				if err != nil {
					klog.Info("Failed to unmarshal Kubernetes resource, err:", err)
					break
				}

				if t.APIVersion == "" || t.Kind == "" {
					klog.V(debugLevel).Info("Not a Kubernetes resource")
				} else if err := sync.handleSingleDeployable(chn, f, file, t, newDplList); err != nil {
					klog.Error(errors.Cause(err).Error())
				}
			}
		}
	}
}

func (sync *ChannelSynchronizer) handleSingleDeployable(
	chn *chv1.Channel,
	fileinfo os.FileInfo,
	filecontent []byte,
	t kubeResource,
	newDplList map[string]*dplv1alpha1.Deployable) error {
	klog.V(debugLevel).Info("Kubernetes resource of kind ", t.Kind, " Creating a deployable.")

	obj := &unstructured.Unstructured{}

	if err := yaml.Unmarshal(filecontent, &obj); err != nil {
		return errors.Wrap(err, "failed to unmarshal Kubernetes resource")
	}

	dpl := &dplv1alpha1.Deployable{}
	dpl.Name = strings.ToLower(chn.GetName() + "-" + t.APIVersion + "-" + t.Kind + "-" + t.Metadata.Namespace + "-" + t.Metadata.Name)
	dpl.Namespace = chn.GetNamespace()

	if err := controllerutil.SetControllerReference(chn, dpl, sync.Scheme); err != nil {
		return errors.Wrap(err, "failed to set controller reference")
	}

	dplanno := make(map[string]string)
	dplanno[dplv1alpha1.AnnotationExternalSource] = fileinfo.Name()
	dplanno[dplv1alpha1.AnnotationLocal] = "false"
	dplanno[dplv1alpha1.AnnotationDeployableVersion] = t.APIVersion
	dpl.SetAnnotations(dplanno)
	dpl.Spec.Template = &runtime.RawExtension{}

	var err error
	dpl.Spec.Template.Raw, err = json.Marshal(obj)

	if err != nil {
		return errors.Wrap(err, "failed to marshal helm CR to template")
	}

	newDplList[dpl.Name] = dpl

	if err := sync.kubeClient.Create(context.TODO(), dpl); err != nil {
		return errors.Wrap(err, "failed to create deployable")
	}

	return nil
}

func (sync *ChannelSynchronizer) syncChannel(chn *chv1.Channel) {
	if chn == nil {
		return
	}

	// Clone the Git repo
	idx, resourceDirs, err := utils.CloneGitRepo(chn, sync.kubeClient)
	if err != nil {
		klog.Error("Failed to clone the git repo: ", err.Error())
		return
	}

	klog.V(debugLevel).Info("There are ", len(idx.Entries), " entries in the index.yaml")
	klog.V(debugLevel).Info("There are ", len(resourceDirs), " non-helm directories.")

	newDplList := make(map[string]*dplv1alpha1.Deployable)
	// sync kube resource deployables
	for _, dir := range resourceDirs {
		files, err := ioutil.ReadDir(dir)
		if err != nil {
			klog.Error("Failed to list files in directory ", dir, err.Error())
		}

		sync.processYamlFile(chn, files, dir, newDplList)
	}

	// chartname, chartversion, exists
	generalmap := make(map[string]map[string]bool) // chartname, chartversion, false
	majorversion := make(map[string]string)        // chartname, chartversion

	for k, cv := range idx.Entries {
		klog.V(debugLevel).Info("Key: ", k)

		chartmap := make(map[string]bool)

		for _, chart := range cv {
			klog.V(debugLevel).Info("Chart:", chart.Name, " Version:", chart.Version)

			chartmap[chart.Version] = false
		}

		generalmap[k] = chartmap
		majorversion[k] = cv[0].Version
	}

	// retrieve deployable list in current channel namespace
	listopt := &client.ListOptions{Namespace: chn.Namespace}
	dpllist := &dplv1alpha1.DeployableList{}

	err = sync.kubeClient.List(context.TODO(), dpllist, listopt)
	if err != nil {
		klog.Info("Error in listing deployables in channel namespace: ", chn.Namespace)
		return
	}

	for _, dpl := range dpllist.Items {
		if err := sync.handleHelmDeployable(dpl, chn, generalmap); err != nil {
			klog.Errorf(errors.Cause(err).Error())
		}

		if err := sync.handleResourceDeployable(dpl, chn, newDplList); err != nil {
			klog.Errorf(errors.Cause(err).Error())
		}
	}

	sync.processGeneralMap(idx, chn, majorversion, generalmap)
}

func (sync *ChannelSynchronizer) handleResourceDeployable(
	dpl dplv1alpha1.Deployable,
	chn *chv1.Channel,
	newDplList map[string]*dplv1alpha1.Deployable) error {
	klog.V(debugLevel).Infof("Synchronizing kube resource deployable %v", dpl.Name)

	if dpl.Spec.Template == nil {
		return errors.New("missing template in deployable:" + dpl.GetName())
	}

	dpltpl := &unstructured.Unstructured{}
	err := json.Unmarshal(dpl.Spec.Template.Raw, dpltpl)

	if err != nil {
		klog.Warning("Failed to unmarshall the template from existing deployable:", dpl, err)
		return errors.New("failed to unmarshall the template from existing deployable")
	}

	if dpltpl.GetKind() == utils.HelmCRKind && dpltpl.GetAPIVersion() == utils.HelmCRAPIVersion {
		klog.Info("Skipping helm release deployable:", dpltpl.GetKind(), ".", dpltpl.GetAPIVersion())
		return nil
	}

	// Delete deployables that don't exist in the bucket anymore
	if _, ok := newDplList[dpl.Name]; !ok {
		klog.Info("Sync - Deleting deployable ", dpl.Namespace, "/", dpl.Name, " from channel ", chn.Name)

		if err := sync.kubeClient.Delete(context.TODO(), &dpl); err != nil {
			return errors.Wrap(err, fmt.Sprintf("sync failed to delete deployable %v", dpl.Name))
		}
	}

	// Update deployables if the template is updated in the git repo
	newdpl := newDplList[dpl.Name]
	newdpltpl := &unstructured.Unstructured{}
	err = json.Unmarshal(newdpl.Spec.Template.Raw, newdpltpl)

	if err != nil {
		klog.Warning("Failed to unmarshall the template from updated deployable:", newdpl, err)
		return errors.Wrap(err, fmt.Sprintf("failed to unmarshall the template from updated deployable: %v", newdpl))
	}

	if !reflect.DeepEqual(dpltpl, newdpltpl) {
		klog.Info("Sync - Updating the template in existing deployable ", dpl.Name, " in channel ", chn.Name)
		dpl.Spec.Template.Raw, err = json.Marshal(newdpltpl)

		if err != nil {
			klog.Warning(err.Error())
			return errors.Wrap(err, fmt.Sprintf("failed to mashall template %v", newdpltpl))
		}

		if err := sync.kubeClient.Update(context.TODO(), &dpl); err != nil {
			return errors.Wrap(err, fmt.Sprintf("failed to update deployable %v", dpl.Name))
		}
	}

	return nil
}

func (sync *ChannelSynchronizer) handleHelmDeployable(dpl dplv1alpha1.Deployable, chn *chv1.Channel, generalmap map[string]map[string]bool) error {
	klog.V(debugLevel).Infof("Synchronizing helm release deployable %v", dpl.Name)

	obj := &helmTemplate{}

	if err := json.Unmarshal(dpl.Spec.Template.Raw, obj); err != nil {
		return errors.New(fmt.Sprintf("failed to unmarshal deployable %v with err %v", dpl, err))
	}

	if obj.Kind != utils.HelmCRKind || obj.APIVersion != utils.HelmCRAPIVersion {
		return errors.New(fmt.Sprintf("Skipping non helm chart deployable %v.%v", obj.Kind, obj.APIVersion))
	}

	cname := obj.Spec.ChartName
	cver := obj.Spec.Version

	keep := false
	chmap := generalmap[cname]

	if cname != "" || cver != "" {
		if chmap != nil {
			if _, ok := chmap[cver]; ok {
				klog.Info("keeping it ", cname, cver)

				keep = true
			}
		}
	}

	if !keep {
		klog.Info("deleting it ", cname, cver)

		if err := sync.kubeClient.Delete(context.TODO(), &dpl); err != nil {
			return errors.Wrap(err, fmt.Sprintf("failed to delete deployable %v", dpl.Name))
		}
	} else {
		chmap[cver] = true
		var crepo string
		if strings.EqualFold(string(chn.Spec.Type), chnv1alpha1.ChannelTypeGitHub) {
			crepo = obj.Spec.Source.GitHub.URLs[0]
		} else {
			crepo = obj.Spec.Source.HelmRepo.URLs[0]
		}

		if crepo != chn.Spec.Pathname {
			klog.Infof("channel %v path changed to %v", chn.GetName(), chn.Spec.Pathname)
			//here might need to do a better code review to see if `crepo = chn.Spec.Pathname` is needed
			if err := sync.kubeClient.Update(context.TODO(), &dpl); err != nil {
				return errors.Wrap(err, fmt.Sprintf("failed to update deployable %v in helm repo channel %v", dpl.Name, chn.Spec.Pathname))
			}
		}
	}

	return nil
}

func (sync *ChannelSynchronizer) processGeneralMap(idx *repo.IndexFile, chn *chv1.Channel,
	majorversion map[string]string, generalmap map[string]map[string]bool) {
	for k, charts := range generalmap {
		mv := majorversion[k]
		obj := &unstructured.Unstructured{}
		obj.SetKind(utils.HelmCRKind)
		obj.SetAPIVersion(utils.HelmCRAPIVersion)
		obj.SetName(k)

		spec := &helmSpec{}
		spec.ChartName = k
		spec.ReleaseName = k
		spec.Version = mv

		sourceurls := &sourceURLs{}
		sourceurls.URLs = []string{chn.Spec.Pathname}

		src := &source{}

		if strings.EqualFold(string(chn.Spec.Type), chnv1alpha1.ChannelTypeGitHub) {
			src.Type = chnv1alpha1.ChannelTypeGitHub
			src.GitHub = sourceurls
			chartVersion, _ := idx.Get(k, mv)
			src.GitHub.ChartPath = chartVersion.URLs[0]
		} else {
			src.Type = chnv1alpha1.ChannelTypeHelmRepo
			src.HelmRepo = sourceurls
		}

		spec.Source = src

		obj.Object["spec"] = spec

		if synced := charts[mv]; !synced {
			klog.Info("generating deployable")

			dpl := &dplv1alpha1.Deployable{}
			dpl.Name = chn.GetName() + "-" + obj.GetName() + "-" + mv
			dpl.Namespace = chn.GetNamespace()

			err := controllerutil.SetControllerReference(chn, dpl, sync.Scheme)
			if err != nil {
				klog.Info("Failed to set controller reference, err:", err)
				break
			}

			dplanno := make(map[string]string)
			dplanno[dplv1alpha1.AnnotationExternalSource] = k
			dplanno[dplv1alpha1.AnnotationLocal] = "false"
			dplanno[dplv1alpha1.AnnotationDeployableVersion] = mv
			dpl.SetAnnotations(dplanno)
			dpl.Spec.Template = &runtime.RawExtension{}
			dpl.Spec.Template.Raw, err = json.Marshal(obj)

			if err != nil {
				klog.Info("Failed to marshal helm cr to template, err:", err)
				break
			}

			err = sync.kubeClient.Create(context.TODO(), dpl)

			if err != nil {
				klog.Info("Failed to create helmrelease deployable, err:", err)
			}

			klog.Info("creating deployable ", k)
		}
	}
}
