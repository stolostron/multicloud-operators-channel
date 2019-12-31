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
	"crypto/tls"
	"io/ioutil"
	"net/http"

	"github.com/ghodss/yaml"
	"k8s.io/helm/pkg/repo"
	"k8s.io/klog"
)

var (
	// HelmCRKind is kind of the Helm CR
	HelmCRKind = "HelmRelease"
	// HelmCRAPIVersion is APIVersion of the Helm CR
	HelmCRAPIVersion = "app.ibm.com/v1alpha1"
	// HelmCRChartName is spec.ChartName of the Helm CR
	HelmCRChartName = "chartName"
	// HelmCRReleaseName is spec.ReleaseName of the Helm CR
	HelmCRReleaseName = "releaseName"
	// HelmCRVersion is spec.Version of the Helm CR
	HelmCRVersion = "version"
	// HelmCRSource is spec.Source of the Helm CR
	HelmCRSource = "source"
	// HelmCRSourceType is spec.Source.Type of the Helm CR
	HelmCRSourceType = "type"
	// HelmCRSourceHelm is spec.Source.Helmrepo of the Helm CR
	HelmCRSourceHelm = "helmrepo"
	// HelmCRSourceGit is spec.Source.Github of the Helm CR
	HelmCRSourceGit = "github"
	// HelmCRRepoURL is spec.Source.Github.Urls or spec.Source.Helmrepo.Urls of the Helm CR
	HelmCRRepoURL = "urls"
	// HelmCRGitRepoChartPath is spec.Source.Github.ChartPath of the Helm CR
	HelmCRGitRepoChartPath = "chartPath"
)

func decideHTTPClient(repoURL string) (*http.Client, error) {
	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	transport := &http.Transport{TLSClientConfig: tlsConfig}
	client := &http.Client{Transport: transport}

	return client, nil
}

func buildRepoURL(repoURL string) string {
	validURL := repoURL

	if validURL[len(repoURL)-1:] != "/" {
		validURL = validURL + "/"
	}

	return validURL + "index.yaml"
}

// GetHelmRepoIndex get the index file from helm repository
func GetHelmRepoIndex(channelPathName string) (*repo.IndexFile, error) {
	repoURL := buildRepoURL(channelPathName)

	client, err := decideHTTPClient(repoURL)
	if err != nil {
		klog.Error(err, "Failed to decide http protocol ", repoURL)
		return nil, err
	}

	resp, err := client.Get(repoURL)
	if err != nil {
		klog.Error(err, "Failed to contact repo: ", repoURL)
		return nil, err
	}

	defer resp.Body.Close()
	klog.V(10).Info("Done retrieving URL: ", repoURL)

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		klog.Error(err, "Unable to read body", repoURL)
		return nil, err
	}

	klog.V(10).Info("Index file: \n", string(body))

	i := &repo.IndexFile{}
	err = yaml.Unmarshal(body, i)

	return i, err
}
