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
	"net/http"
	"testing"
)

const (
	helmTests     = "../../tests/helm/testhelm"
	helmChartsNum = 1
)

func loadLocalIdx(idxPath string) (*http.Response, error) {
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

func TestGetHelmRepoIndex(t *testing.T) {
	idx, err := GetHelmRepoIndex(helmTests, loadLocalIdx)

	if err != nil {
		t.Errorf("failed to clone %+v", err)
	}

	if len(idx.Entries) != helmChartsNum {
		t.Errorf("faild to parse helm chart, wanted %v, got %v", helmChartsNum, len(idx.Entries))
	}
}
