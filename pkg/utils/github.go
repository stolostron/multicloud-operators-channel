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
	"github.com/pkg/errors"
	"k8s.io/helm/pkg/chartutil"
	"k8s.io/helm/pkg/repo"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"
)

const (
	// UserID is key of GitHub user ID in secret
	UserID = "user"
	// Password is key of GitHub user password or personal token in secret
	Password = "accessToken"
)

type cred struct {
	accessKey *string
	pwd       *string
}

func fetchCreditionalOfGitHub(chn *chv1.Channel, c client.Client) (*cred, error) {
	secret := &corev1.Secret{}
	secns := chn.Spec.SecretRef.Namespace

	if secns == "" {
		secns = chn.Namespace
	}

	if err := c.Get(context.TODO(), types.NamespacedName{Name: chn.Spec.SecretRef.Name, Namespace: secns}, secret); err != nil {
		return nil, errors.Wrap(err, "unable to get secret.")
	}

	gitCred := &cred{}
	if err := yaml.Unmarshal(secret.Data[UserID], gitCred.accessKey); err != nil {
		return nil, errors.Wrap(err, "unable to unmarshal github access key")
	}

	if err := yaml.Unmarshal(secret.Data[Password], gitCred.pwd); err != nil {
		return nil, errors.Wrap(err, "unable to unmarshal github secert key")
	}

	return gitCred, nil
}

func createTmpDir(ns, name string) (string, error) {
	repoRoot := filepath.Join(os.TempDir(), ns, name)
	if _, err := os.Stat(repoRoot); os.IsNotExist(err) {
		if err := os.MkdirAll(repoRoot, os.ModePerm); err != nil {
			klog.Error(err, "unable to unable to unmarshal secret.")
			return "", errors.Wrap(err, "unable to create a temp dirctory")
		}
	} else {
		if err := os.RemoveAll(repoRoot); err != nil {
			return "", errors.Wrap(err, "unable to clean up temp dirctory")
		}
	}

	return repoRoot, nil
}

type CloneFunc = func(string, bool, *git.CloneOptions) (*git.Repository, error)

// CloneGitRepo clones the GitHub repo
func CloneGitRepo(chn *chv1.Channel, kubeClient client.Client, cOpt ...CloneFunc) (*repo.IndexFile, map[string]string, error) {
	var cFunc CloneFunc
	if len(cOpt) == 0 {
		cFunc = git.PlainClone
	} else {
		cFunc = cOpt[0]
	}

	options := &git.CloneOptions{
		URL:               chn.Spec.Pathname,
		Depth:             1,
		SingleBranch:      true,
		RecurseSubmodules: git.DefaultSubmoduleRecursionDepth,
		ReferenceName:     plumbing.Master,
	}

	if chn.Spec.SecretRef != nil {
		gitCred, err := fetchCreditionalOfGitHub(chn, kubeClient)
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to clone git")
		}

		options.Auth = &githttp.BasicAuth{
			Username: *gitCred.accessKey,
			Password: *gitCred.pwd,
		}
	}

	repoRoot, err := createTmpDir(chn.Namespace, chn.Name)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to clone git")
	}

	klog.V(debugLevel).Info("Cloning ", chn.Spec.Pathname, " into ", repoRoot)

	if _, err := cFunc(repoRoot, false, options); err != nil {
		return nil, nil, errors.Wrap(err, "failed to clone git")
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
