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

package utils

import (
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	githttp "gopkg.in/src-d/go-git.v4/plumbing/transport/http"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"

	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/ghodss/yaml"
	"k8s.io/helm/pkg/chartutil"
	"k8s.io/helm/pkg/repo"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	chnv1alpha1 "github.com/IBM/multicloud-operators-channel/pkg/apis/app/v1alpha1"
)

const (
	// UserID is key of GitHub user ID in secret
	UserID = "user"
	// Password is key of GitHub user password or personal token in secret
	Password = "password"
)

// CloneGitRepo clones the GitHub repo
func CloneGitRepo(chn *chnv1alpha1.Channel, kubeClient client.Client) (*repo.IndexFile, map[string]string, error) {
	options := &git.CloneOptions{
		URL:               chn.Spec.PathName,
		Depth:             1,
		SingleBranch:      true,
		RecurseSubmodules: git.DefaultSubmoduleRecursionDepth,
		ReferenceName:     plumbing.Master,
	}

	if chn.Spec.SecretRef != nil {
		secret := &corev1.Secret{}
		secns := chn.Spec.SecretRef.Namespace

		if secns == "" {
			secns = chn.Namespace
		}

		err := kubeClient.Get(context.TODO(), types.NamespacedName{Name: chn.Spec.SecretRef.Name, Namespace: secns}, secret)

		if err != nil {
			klog.Error(err, "Unable to get secret.")
			return nil, nil, err
		}

		username := ""
		password := ""

		err = yaml.Unmarshal(secret.Data[UserID], &username)
		if err != nil {
			klog.Error(err, "Unable to unable to unmarshal secret.")
			return nil, nil, err
		}

		err = yaml.Unmarshal(secret.Data[Password], &password)
		if err != nil {
			klog.Error(err, "unable to unable to unmarshal secret.")
			return nil, nil, err
		}

		options.Auth = &githttp.BasicAuth{
			Username: username,
			Password: password,
		}
	}

	repoRoot := filepath.Join(os.TempDir(), chn.Namespace, chn.Name)
	if _, err := os.Stat(repoRoot); os.IsNotExist(err) {
		err := os.MkdirAll(repoRoot, os.ModePerm)
		if err != nil {
			klog.Error(err, "unable to unable to unmarshal secret.")
			return nil, nil, err
		}
	} else {
		err := os.RemoveAll(repoRoot)
		if err != nil {
			klog.Error(err, "unable to unable to unmarshal secret.")
			return nil, nil, err
		}
	}

	klog.V(debugLevel).Info("Cloning ", chn.Spec.PathName, " into ", repoRoot)
	_, err := git.PlainClone(repoRoot, false, options)

	if err != nil {
		klog.Error("Failed to git clone: ", err.Error())
		return nil, nil, err
	}

	// Generate index.yaml
	indexFile, resourceDirs, err := generateIndexYAML(repoRoot)
	if err != nil {
		klog.Error("Failed to generate helm chart index file", err.Error())
		return nil, nil, err
	}
	// Generate list of resource dirs
	return indexFile, resourceDirs, nil
}

func generateIndexYAML(repoRoot string) (*repo.IndexFile, map[string]string, error) {
	// In the cloned git repo root, find all helm chart directories
	chartDirs := make(map[string]string)
	// In the cloned git repo root, also find all non-helm-chart directories
	resourceDirs := make(map[string]string)

	currentChartDir := "NONE"
	err := filepath.Walk(repoRoot,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				klog.V(debugLevel).Info("Ignoring subfolders of ", currentChartDir)
				if _, err := os.Stat(path + "/Chart.yaml"); err == nil {
					klog.V(debugLevel).Info("Found Chart.yaml in ", path)
					if !strings.HasPrefix(path, currentChartDir) {
						klog.V(debugLevel).Info("This is a helm chart folder.")
						chartDirs[path+"/"] = path + "/"
						currentChartDir = path + "/"
					}
				} else if !strings.HasPrefix(path, currentChartDir) && !strings.HasPrefix(path, repoRoot+"/.git") {
					klog.V(debugLevel).Info("This is not a helm chart directory. ", path)
					resourceDirs[path+"/"] = path + "/"
				}
			}
			return nil
		})

	if err != nil {
		return nil, nil, err
	}

	// Build a helm repo index file
	indexFile := repo.NewIndexFile()

	for chartDir := range chartDirs {
		chartFolderName := filepath.Base(chartDir)
		chartParentDir := strings.Split(chartDir, chartFolderName)[0]
		// Get the relative parent directory from the git repo root
		chartBaseDir := strings.SplitAfter(chartParentDir, repoRoot+"/")[1]
		chartMetadata, err := chartutil.LoadChartfile(chartDir + "Chart.yaml")

		if err != nil {
			klog.Error("There was a problem in generating helm charts index file: ", err.Error())
			return nil, nil, err
		}

		indexFile.Add(chartMetadata, chartFolderName, chartBaseDir, "generated-by-multicloud-operators-subscription")
	}

	indexFile.SortEntries()
	b, err := yaml.Marshal(indexFile)
	if err != nil {
		return nil, nil, err
	}

	klog.V(debugLevel).Info("New index file ", string(b))

	return indexFile, resourceDirs, nil
}
