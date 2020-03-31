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

func Test_ParseKubeResoures(t *testing.T) {
	testYaml1 := `---
---
---
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-configmap-1
  namespace: default
data:
  path: resource
---
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-configmap-2
  namespace: default
data:
  path: resource
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-configmap-3
  namespace: default
data:
  path: resource
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-configmap-4
  namespace: default
data:
  path: resource
---`
	ret := ParseKubeResoures([]byte(testYaml1))

	if len(ret) != 4 {
		t.Errorf("faild to parse yaml objects, wanted %v, got %v", 4, len(ret))
	}

	testYaml2 := `---
---
---
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-configmap-1
  namespace: default
data:
  path: resource
---
---`
	ret = ParseKubeResoures([]byte(testYaml2))

	if len(ret) != 1 {
		t.Errorf("faild to parse yaml objects, wanted %v, got %v", 1, len(ret))
	}

	testYaml3 := `---
---
---
---
apiVersiondfdfd: v1
kinddfdfdfdf: ConfigMap
metadata:
  name: test-configmap-1
  namespace: default
data:
  path: resource
---
---`
	ret = ParseKubeResoures([]byte(testYaml3))

	if len(ret) != 0 {
		t.Errorf("faild to parse yaml objects, wanted %v, got %v", 0, len(ret))
	}

	testYaml4 := `---
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-configmap-1
  namespace: default
data:
  path: resource`
	ret = ParseKubeResoures([]byte(testYaml4))

	if len(ret) != 1 {
		t.Errorf("faild to parse yaml objects, wanted %v, got %v", 1, len(ret))
	}

	testYaml5 := `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-configmap-1
  namespace: default
data:
  path: resource`
	ret = ParseKubeResoures([]byte(testYaml5))

	if len(ret) != 1 {
		t.Errorf("faild to parse yaml objects, wanted %v, got %v", 1, len(ret))
	}
}
