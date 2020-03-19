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
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/helm/pkg/repo"
	"k8s.io/klog"
)

const InsecureSkipVerifyFlag = "insecureSkipVerify"

func decideHTTPClient(repoURL string, chnRefCfgMap *corev1.ConfigMap) *http.Client {
	klog.V(infoLevel).Info(repoURL)

	tlsConfig := &tls.Config{}

	if chnRefCfgMap != nil && chnRefCfgMap.Data[InsecureSkipVerifyFlag] != "" {
		b, err := strconv.ParseBool(chnRefCfgMap.Data[InsecureSkipVerifyFlag])
		if err != nil {
			klog.Warning("unable to parse insecureSkipVerify false, using default value: false")
		}
		tlsConfig.InsecureSkipVerify = b
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	return client
}

func buildRepoURL(repoURL string) string {
	validURL := repoURL

	if validURL[len(repoURL)-1:] != "/" {
		validURL += "/"
	}

	return validURL + "index.yaml"
}

func GetChartIndex(chnPathname string, chnRefCfgMap *corev1.ConfigMap) (*http.Response, error) {
	repoURL := buildRepoURL(chnPathname)

	client := decideHTTPClient(repoURL, chnRefCfgMap)

	resp, err := client.Get(repoURL)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to contact repo: %v", repoURL))
	}

	return resp, nil
}

type LoadIndexPageFunc func(string, *corev1.ConfigMap) (*http.Response, error)

func LoadLocalIdx(idxPath string, cfg *corev1.ConfigMap) (*http.Response, error) {
	localDir := http.Dir(idxPath)
	content, err := localDir.Open("index.yaml")

	if err != nil {
		return nil, err
	}

	resp := &http.Response{
		Body: content,
	}

	return resp, nil
}

// GetHelmRepoIndex get the index file from helm repository
func GetHelmRepoIndex(channelPathName string, chnRefCfgMap *corev1.ConfigMap, loadIdx LoadIndexPageFunc) (*repo.IndexFile, error) {
	resp, err := loadIdx(channelPathName, chnRefCfgMap)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get chart index")
	}

	defer resp.Body.Close()

	klog.V(debugLevel).Info("Done retrieving URL: ", buildRepoURL(channelPathName))

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("unable to read body of %v", buildRepoURL(channelPathName)))
	}

	klog.V(debugLevel).Info("Index file: \n", string(body))

	i := &repo.IndexFile{}
	if err := yaml.Unmarshal(body, i); err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("unable to unmarshal repo %v", buildRepoURL(channelPathName)))
	}

	return i, nil
}
