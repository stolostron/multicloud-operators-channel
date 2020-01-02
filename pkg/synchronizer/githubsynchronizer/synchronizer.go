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
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	"github.com/ghodss/yaml"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	chnv1alpha1 "github.com/IBM/multicloud-operators-channel/pkg/apis/app/v1alpha1"
	"github.com/IBM/multicloud-operators-channel/pkg/utils"
	dplv1alpha1 "github.com/IBM/multicloud-operators-deployable/pkg/apis/app/v1alpha1"
	deputils "github.com/IBM/multicloud-operators-deployable/pkg/utils"
)

// ChannelSynchronizer syncs github channels with github repository
type ChannelSynchronizer struct {
	Scheme       *runtime.Scheme
	kubeClient   client.Client
	Signal       <-chan struct{}
	SyncInterval int
	ChannelMap   map[types.NamespacedName]*chnv1alpha1.Channel
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
}

// CreateSynchronizer - creates an instance of ChannelSynchronizer
func CreateSynchronizer(config *rest.Config, scheme *runtime.Scheme, syncInterval int) (*ChannelSynchronizer, error) {
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
		klog.V(10).Info("synching channel ", ch.Name)
		sync.syncChannel(ch)
	}
}

func (sync *ChannelSynchronizer) syncChannel(chn *chnv1alpha1.Channel) {
	if chn == nil {
		return
	}

	// Clone the Git repo
	idx, resourceDirs, err := utils.CloneGitRepo(chn, sync.kubeClient)
	if err != nil {
		klog.Error("Failed to clone the git repo: ", err.Error())
		return
	}

	klog.V(10).Info("There are ", len(idx.Entries), " entries in the index.yaml")
	klog.V(10).Info("There are ", len(resourceDirs), " non-helm directories.")

	// sync kube resource deployables
	for _, dir := range resourceDirs {
		files, err := ioutil.ReadDir(dir)
		if err != nil {
			klog.Error("Failed to list files in directory ", dir, err.Error())
		}

		for _, f := range files {
			// If YAML or YML,
			if f.Mode().IsRegular() {
				if strings.EqualFold(filepath.Ext(f.Name()), ".yml") || strings.EqualFold(filepath.Ext(f.Name()), ".yaml") {
					// check it it is Kubernetes resource
					klog.V(10).Info("scanning file ", f.Name())
					file, _ := ioutil.ReadFile(filepath.Join(dir, f.Name()))
					t := kubeResource{}

					err = yaml.Unmarshal(file, &t)
					if err != nil {
						klog.Info("Failed to unmarshal Kubernetes resource, err:", err)
						break
					}

					if t.APIVersion == "" || t.Kind == "" {
						klog.V(10).Info("Not a Kubernetes resource")
					} else {
						klog.V(10).Info("Kubernetes resource of kind ", t.Kind, " Creating a deployable.")

						obj := &unstructured.Unstructured{}

						err = yaml.Unmarshal(file, &obj)
						if err != nil {
							klog.Info("Failed to unmarshal Kubernetes resource, err:", err)
							break
						}

						dpl := &dplv1alpha1.Deployable{}
						dpl.Name = strings.ToLower(chn.GetName() + "-" + t.Kind + "-deployable")
						dpl.Namespace = chn.GetNamespace()

						err = controllerutil.SetControllerReference(chn, dpl, sync.Scheme)
						if err != nil {
							klog.Info("Failed to set controller reference, err:", err)
							break
						}

						dplanno := make(map[string]string)
						dplanno[dplv1alpha1.AnnotationExternalSource] = f.Name()
						dplanno[dplv1alpha1.AnnotationLocal] = "false"
						dplanno[dplv1alpha1.AnnotationDeployableVersion] = t.APIVersion
						dpl.SetAnnotations(dplanno)
						dpl.Spec.Template = &runtime.RawExtension{}
						dpl.Spec.Template.Raw, err = json.Marshal(obj)
						if err != nil {
							klog.Info("Failed to marshal helm CR to template, err:", err)
							break
						}
						err = sync.kubeClient.Create(context.TODO(), dpl)
						if err != nil {
							klog.Info("Failed to create helmrelease deployable, err:", err)
						}
					}
				}
			}
		}
	}

	// chartname, chartversion, exists
	generalmap := make(map[string]map[string]bool) // chartname, chartversion, false
	majorversion := make(map[string]string)        // chartname, chartversion

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
	dpllist := &dplv1alpha1.DeployableList{}

	err = sync.kubeClient.List(context.TODO(), dpllist, listopt)
	if err != nil {
		klog.Info("Error in listing deployables in channel namespace: ", chn.Namespace)
		return
	}

	for _, dpl := range dpllist.Items {
		klog.V(10).Info("synching dpl ", dpl.Name)
		//obj := &unstructured.Unstructured{}
		obj := &helmTemplate{}
		err := json.Unmarshal(dpl.Spec.Template.Raw, obj)

		if err != nil {
			klog.Warning("Processing local deployable with error template:", dpl, err)

			continue
		}

		if obj.Kind != utils.HelmCRKind || obj.APIVersion != utils.HelmCRAPIVersion {
			klog.Info("Skipping non helm chart deployable:", obj.Kind, ".", obj.APIVersion)

			continue
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

			err = sync.kubeClient.Delete(context.TODO(), &dpl)
			if err != nil {
				klog.Errorf("Failed to delete deployable %v, due to  err: %v", dpl.Name, err)
				break
			}
		} else {
			chmap[cver] = true
			var crepo string
			if strings.EqualFold(string(chn.Spec.Type), chnv1alpha1.ChannelTypeGitHub) {
				crepo = obj.Spec.Source.GitHub.URLs[0]
			} else {
				crepo = obj.Spec.Source.HelmRepo.URLs[0]
			}
			if crepo != chn.Spec.PathName {
				klog.Info("path changed ")
				//crepo = chn.Spec.PathName
				err = sync.kubeClient.Update(context.TODO(), &dpl)
				if err != nil {
					klog.Error("Failed to update deployable in helm repo channel:", dpl.Name, " to ", chn.Spec.PathName)
				}
			}
		}
	}

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
		sourceurls.URLs = []string{chn.Spec.PathName}

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

			err = controllerutil.SetControllerReference(chn, dpl, sync.Scheme)
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
