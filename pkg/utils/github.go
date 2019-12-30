// Licensed Materials - Property of IBM
// (c) Copyright IBM Corporation 2016, 2019. All Rights Reserved.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP  Schedule Contract with IBM Corp.

package utils

import (
	//"github.com/google/go-github/github"
	//"golang.org/x/oauth2"
	"github.com/golang/glog"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	githttp "gopkg.in/src-d/go-git.v4/plumbing/transport/http"
	"k8s.io/apimachinery/pkg/types"

	//"io/ioutil"
	"os"
	//"k8s.io/helm/pkg/proto/hapi/chart"
	"context"
	"path/filepath"
	"strings"

	"github.com/ghodss/yaml"
	"k8s.io/helm/pkg/chartutil"
	"k8s.io/helm/pkg/repo"

	chnv1alpha1 "github.com/IBM/multicloud-operators-channel/pkg/apis/app/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
			glog.Error(err, "Unable to get secret.")
			return nil, nil, err
		}

		username := ""
		password := ""
		yaml.Unmarshal(secret.Data[UserID], &username)
		yaml.Unmarshal(secret.Data[Password], &password)

		options.Auth = &githttp.BasicAuth{
			Username: username,
			Password: password,
		}
	}

	repoRoot := filepath.Join(os.TempDir(), chn.Namespace, chn.Name)
	if _, err := os.Stat(repoRoot); os.IsNotExist(err) {
		os.MkdirAll(repoRoot, os.ModePerm)
	} else {
		os.RemoveAll(repoRoot)
	}

	glog.V(10).Info("Cloning ", chn.Spec.PathName, " into ", repoRoot)
	_, err := git.PlainClone(repoRoot, false, options)
	if err != nil {
		glog.Error("Failed to git clone: ", err.Error())
		return nil, nil, err
	}

	// Generate index.yaml
	indexFile, resourceDirs, err := generateIndexYAML(repoRoot)
	if err != nil {
		glog.Error("Failed to generate helm chart index file", err.Error())
	}
	// Generate list of resource dirs
	return indexFile, resourceDirs, nil
}

func generateIndexYAML(repoRoot string) (*repo.IndexFile, map[string]string, error) {

	// In the cloned git repo root, find all helm chart directories
	chartDirs := make(map[string]string)
	// In the cloned git repo root, also find all non-helm-chart directories
	resourceDirs := make(map[string]string)
	var currentChartDir string
	currentChartDir = "NONE"
	err := filepath.Walk(repoRoot,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				glog.V(10).Info("Ignoring subfolders of ", currentChartDir)
				if _, err := os.Stat(path + "/Chart.yaml"); err == nil {
					glog.V(10).Info("Found Chart.yaml in ", path)
					if !strings.HasPrefix(path, currentChartDir) {
						glog.V(10).Info("This is a helm chart folder.")
						chartDirs[path+"/"] = path + "/"
						currentChartDir = path + "/"
					}
				} else {
					if !strings.HasPrefix(path, currentChartDir) && !strings.HasPrefix(path, repoRoot+"/.git") {
						glog.V(10).Info("This is not a helm chart directory. ", path)
						resourceDirs[path+"/"] = path + "/"
					}
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
			glog.Error("There was a problem in generating helm charts index file: ", err.Error())
			return nil, nil, err
		}
		indexFile.Add(chartMetadata, chartFolderName, chartBaseDir, "generated-by-multicloud-operators-subscription")
	}
	indexFile.SortEntries()
	b, _ := yaml.Marshal(indexFile)
	glog.V(10).Info("New index file ", string(b))
	return indexFile, resourceDirs, nil
}
