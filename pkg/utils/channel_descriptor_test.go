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

	chnv1alpha1 "github.com/IBM/multicloud-operators-channel/pkg/apis/app/v1alpha1"
	"github.com/IBM/multicloud-operators-channel/pkg/utils"
	"github.com/google/go-cmp/cmp"
	"github.com/onsi/gomega"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type myObjectStore struct {
	Clt map[string]map[string]string
}

func (m *myObjectStore) InitObjectStoreConnection(endpoint, accessKeyID, secretAccessKey string) error {
	m.Clt = make(map[string]map[string]string)
	return nil
}

//it's odd that we request the storage to be pre-set
func (m *myObjectStore) Exists(bucket string) error {
	if _, ok := m.Clt[bucket]; !ok {
		m.Create(bucket)
	}
	return nil
}

func (m *myObjectStore) Create(bucket string) error {
	m.Clt[bucket] = make(map[string]string)
	return nil
}

func (m *myObjectStore) List(bucket string) ([]string, error) {
	keys := []string{}

	for k := range m.Clt {
		keys = append(keys, k)
	}
	return keys, nil
}

func (m *myObjectStore) Put(bucket string, dplObj utils.DeployableObject) error {
	m.Clt[bucket] = map[string]string{
		"name": dplObj.Name,
	}
	return nil
}

func (m *myObjectStore) Delete(bucket, name string) error {
	if _, ok := m.Clt[bucket]; !ok {
		return errors.New("empty bucket")
	}

	delete(m.Clt, bucket)

	return nil
}

func (m *myObjectStore) Get(bucket, name string) (utils.DeployableObject, error) {
	if _, ok := m.Clt[bucket]; !ok {
		return utils.DeployableObject{}, errors.New("empty bucket")
	}

	bucketMap := m.Clt[bucket]
	dplObj := utils.DeployableObject{
		Name: bucketMap["name"],
	}

	return dplObj, nil
}

func TestValidateChannel(t *testing.T) {
	testCh := "objch"
	testNs := "ch-obj"
	testSrt := "refered-srt"
	testBucket := "bucket"

	refSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSrt,
			Namespace: testNs,
		},
		Data: map[string][]byte{
			"accessId":  []byte{},
			"secretKey": []byte{},
		},
	}

	testCases := []struct {
		desc       string
		chn        *chnv1alpha1.Channel
		kubeClient client.Client
		myStorage  *myObjectStore
		wanted     *myObjectStore
	}{
		{
			desc: "channel without referred secret",
			chn: &chnv1alpha1.Channel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testCh,
					Namespace: testNs,
				},
				Spec: chnv1alpha1.ChannelSpec{
					Type:     chnv1alpha1.ChannelTypeGitHub,
					PathName: "",
				},
			},
			kubeClient: c,
			wanted:     nil,
			myStorage:  nil,
		},
		{
			desc: "channel with referred secret",
			chn: &chnv1alpha1.Channel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testCh,
					Namespace: testNs,
				},
				Spec: chnv1alpha1.ChannelSpec{
					Type:     chnv1alpha1.ChannelTypeGitHub,
					PathName: "",
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
			chn: &chnv1alpha1.Channel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testCh,
					Namespace: testNs,
				},
				Spec: chnv1alpha1.ChannelSpec{
					Type:     chnv1alpha1.ChannelTypeGitHub,
					PathName: "https://www.google.com/" + testBucket + "/",
					SecretRef: &v1.ObjectReference{
						Kind:      "Secret",
						Name:      testSrt,
						Namespace: testNs,
					},
				},
			},
			kubeClient: c,
			myStorage: &myObjectStore{
				Clt: map[string]map[string]string{
					testBucket: make(map[string]string),
				},
			},
			wanted: &myObjectStore{
				Clt: map[string]map[string]string{
					testBucket: map[string]string{},
				},
			},
		},
	}

	g := gomega.NewGomegaWithT(t)

	g.Expect(c.Create(context.TODO(), refSecret)).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), refSecret)

	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			myChDescriptor, _ := utils.CreateChannelDescriptor()

			if tC.myStorage == nil {
				_ = myChDescriptor.ValidateChannel(tC.chn, tC.kubeClient)
			} else {
				_ = myChDescriptor.ValidateChannel(tC.chn, tC.kubeClient, tC.myStorage)
			}

			if diff := cmp.Diff(tC.myStorage, tC.wanted); diff != "" {
				t.Errorf("(+want, -got)\n%s", diff)
			}
		})
	}
}
