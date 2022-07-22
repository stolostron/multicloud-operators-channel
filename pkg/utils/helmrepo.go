// Copyright 2021 The Kubernetes Authors.
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
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/ghodss/yaml"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/repo"
	corev1 "k8s.io/api/core/v1"
)

const (
	InsecureSkipVerifyFlag = "insecureSkipVerify"
)

func buildRepoURL(repoURL string) string {
	validURL := repoURL

	if validURL[len(repoURL)-1:] != "/" {
		validURL += "/"
	}

	return validURL + "index.yaml"
}

type LoadIndexPageFunc func(idxPath string, secureSkip bool, srt *corev1.Secret, cfg *corev1.ConfigMap, logger logr.Logger) (*http.Response, error)

func LoadLocalIdx(idxPath string, insecureSkipVerify bool, srt *corev1.Secret, cfg *corev1.ConfigMap, lgger logr.Logger) (*http.Response, error) {
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
func GetHelmRepoIndex(
	channelPathName string,
	insecureSkipVerify bool,
	chnRefSrt *corev1.Secret,
	chnRefCfgMap *corev1.ConfigMap,
	loadIdx LoadIndexPageFunc,
	logger logr.Logger) (*repo.IndexFile, error) {
	resp, err := loadIdx(channelPathName, insecureSkipVerify, chnRefSrt, chnRefCfgMap, logger)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get chart index")
	}

	defer resp.Body.Close()

	logger.Info(fmt.Sprint("Done retrieving URL: ", buildRepoURL(channelPathName)))

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("unable to read body of %v", buildRepoURL(channelPathName)))
	}

	logger.V(3).Info(fmt.Sprintf("Index file: %v", string(body)))

	i := &repo.IndexFile{}
	if err := yaml.Unmarshal(body, i); err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("unable to unmarshal repo %v", buildRepoURL(channelPathName)))
	}

	return i, nil
}
