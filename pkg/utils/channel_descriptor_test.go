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

package utils_test

import (
	"context"
	"testing"

	tlog "github.com/go-logr/logr/testing"
	"github.com/google/go-cmp/cmp"
	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"
	"github.com/open-cluster-management/multicloud-operators-channel/pkg/utils"
)

func TestValidateChannel(t *testing.T) {
	testCh := "objch"
	testNs := "ch-obj"
	testSrt := "referred-srt"
	testBucket := "bucket"

	refSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSrt,
			Namespace: testNs,
		},
		Data: map[string][]byte{
			"accessId":  {},
			"secretKey": {},
		},
	}

	testCases := []struct {
		desc       string
		chn        *chv1.Channel
		kubeClient client.Client
		myStorage  *utils.FakeObjectStore
		wanted     *utils.FakeObjectStore
	}{
		{
			desc: "channel without referred secret",
			chn: &chv1.Channel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testCh,
					Namespace: testNs,
				},
				Spec: chv1.ChannelSpec{
					Type:     chv1.ChannelTypeGit,
					Pathname: "",
				},
			},
			kubeClient: c,
			wanted:     nil,
			myStorage:  nil,
		},
		{
			desc: "channel with referred secret",
			chn: &chv1.Channel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testCh,
					Namespace: testNs,
				},
				Spec: chv1.ChannelSpec{
					Type:     chv1.ChannelTypeGit,
					Pathname: "",
					SecretRef: &v1.ObjectReference{
						Kind:      "Secret",
						Name:      testSrt,
						Namespace: testNs,
					},
				},
			},
			kubeClient: c,
			wanted:     nil,
			myStorage:  nil,
		},
		{
			desc: "channel with referred secret and correct pathname with empty storage",
			chn: &chv1.Channel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testCh,
					Namespace: testNs,
				},
				Spec: chv1.ChannelSpec{
					Type:     chv1.ChannelTypeGit,
					Pathname: "https://www.google.com/" + testBucket + "/",
					SecretRef: &v1.ObjectReference{
						Kind:      "Secret",
						Name:      testSrt,
						Namespace: testNs,
					},
				},
			},
			kubeClient: c,
			myStorage: &utils.FakeObjectStore{
				Clt: map[string]map[string]utils.DeployableObject{
					testBucket: make(map[string]utils.DeployableObject),
				},
			},
			wanted: &utils.FakeObjectStore{
				Clt: map[string]map[string]utils.DeployableObject{
					testBucket: {},
				},
			},
		},
	}

	g := gomega.NewGomegaWithT(t)

	g.Expect(c.Create(context.TODO(), refSecret)).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), refSecret)

	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			myChDescriptor, _ := utils.CreateObjectStorageChannelDescriptor()

			if tC.myStorage == nil {
				_ = myChDescriptor.ConnectWithResourceHost(tC.chn, tC.kubeClient, tlog.NullLogger{})
			} else {
				_ = myChDescriptor.ConnectWithResourceHost(tC.chn, tC.kubeClient, tlog.NullLogger{}, tC.myStorage)
			}

			if diff := cmp.Diff(tC.myStorage, tC.wanted); diff != "" {
				t.Errorf("(+want, -got)\n%s", diff)
			}
		})
	}
}
