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

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"k8s.io/helm/pkg/repo"
	"k8s.io/klog"
)

func decideHTTPClient(repoURL string) *http.Client {
	klog.V(infoLevel).Info(repoURL)

	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	transport := &http.Transport{TLSClientConfig: tlsConfig}
	client := &http.Client{Transport: transport}

	return client
}

func buildRepoURL(repoURL string) string {
	validURL := repoURL

	if validURL[len(repoURL)-1:] != "/" {
		validURL += "/"
	}

	return validURL + "index.yaml"
}

func getChartIndex(chnPathname string) (*http.Response, error) {
	repoURL := buildRepoURL(chnPathname)

	client := decideHTTPClient(repoURL)

	resp, err := client.Get(repoURL)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to contact repo: %v", repoURL))
	}

	return resp, nil
}

// GetHelmRepoIndex get the index file from helm repository
func GetHelmRepoIndex(channelPathName string) (*repo.IndexFile, error) {
	resp, err := getChartIndex(channelPathName)
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
