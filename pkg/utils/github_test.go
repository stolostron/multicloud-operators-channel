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
	"testing"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"
)

const (
	gitTests  = "../../tests/github/testrepo"
	chartsNum = 1
	resDirNum = 2
)

func Test_CloneGitRepo(t *testing.T) {
	chnKey := types.NamespacedName{Name: "t-ch", Namespace: "t-ch-ns"}
	chn := &chv1.Channel{
		ObjectMeta: v1.ObjectMeta{
			Namespace: chnKey.Namespace,
			Name:      chnKey.Name,
		},
		Spec: chv1.ChannelSpec{
			Pathname: gitTests,
		},
	}

	_, idx, resDirMap, err := CloneGitRepo(chn, nil, FakeClone)

	if err != nil {
		t.Errorf("failed to clone %+v", err)
	}

	if len(idx.Entries) != chartsNum {
		t.Errorf("faild to parse helm chart, wanted %v, got %v", chartsNum, len(idx.Entries))
	}

	if len(resDirMap) != resDirNum {
		t.Errorf("faild to parse yaml objects, wanted %v, got %v", resDirNum, len(resDirMap))
	}
}
