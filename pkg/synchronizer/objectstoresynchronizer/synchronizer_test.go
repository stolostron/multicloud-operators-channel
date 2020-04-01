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

package objectstoresynchronizer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"
	"github.com/open-cluster-management/multicloud-operators-channel/pkg/utils"
	dplv1 "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis/apps/v1"
)

//1. objectstore is empty, delete all deployables, if there's any
// Test_alginClusterResourceWithHost_emptyHost
//2. if objectstore has entry, then it should be deployed
// Test_alginClusterResourceWithHost_createDplBasedOnHost
//3. some deployables changed at objectstore then the local deployables should be updated as well
// Test_alginClusterResourceWithHost_updateDplBasedOnHost

var _ = Describe("sync channel and object store", func() {
	Context("simple run with fake bucket client", func() {

		It("simple start up", func() {
			chdesc, _ := utils.CreateObjectStorageChannelDescriptor()
			objsync, _ := CreateObjectStoreSynchronizer(k8sManager.GetConfig(), chdesc, 2)
			objsync.ObjectStore = &utils.FakeObjectStore{}
			Expect(objsync.syncChannelsWithObjStore()).ShouldNot(HaveOccurred())
		})

	})

	//empty host
	Context("delete resource on cluster due to empty bucket", func() {

		var (
			tKey = types.NamespacedName{Name: "tch-1", Namespace: "tns-1"}

			srtName = "srt-ref-1"
			refSrt  = &corev1.Secret{
				ObjectMeta: v1.ObjectMeta{
					Name:      srtName,
					Namespace: tKey.Namespace,
				},
				Data: map[string][]byte{
					utils.SecretMapKeyAccessKeyID:     {},
					utils.SecretMapKeySecretAccessKey: {},
				},
			}

			tCh = &chv1.Channel{
				ObjectMeta: v1.ObjectMeta{
					Name:      tKey.Name,
					Namespace: tKey.Namespace,
				},
				Spec: chv1.ChannelSpec{
					Type:     chv1.ChannelType("objectbucket"),
					Pathname: "/" + tKey.Namespace,
					SecretRef: &corev1.ObjectReference{
						Name:      srtName,
						Namespace: tKey.Namespace,
					},
				},
			}

			dplName = "t-dpl-1"
			tDpl    = &dplv1.Deployable{
				ObjectMeta: v1.ObjectMeta{
					Name:      dplName,
					Namespace: tKey.Namespace,
				},
				Spec: dplv1.DeployableSpec{
					Template: &runtime.RawExtension{
						Object: &corev1.ConfigMap{
							TypeMeta: v1.TypeMeta{
								Kind: "ConfigMap",
							},
							ObjectMeta: v1.ObjectMeta{
								Name: "cm-1",
								Annotations: map[string]string{
									dplv1.AnnotationExternalSource: "true",
								},
							},
						},
					},
				},
			}

			objsync *ChannelSynchronizer
		)

		It("should not have deployable on cluster when query given the bucket is empty", func() {
			Expect(k8sClient.Create(context.TODO(), refSrt)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), refSrt)).Should(Succeed())
			}()
			Expect(k8sClient.Create(context.TODO(), tCh)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), tCh)).Should(Succeed())
			}()
			Expect(k8sClient.Create(context.TODO(), tDpl)).Should(Succeed())

			chdesc, _ := utils.CreateObjectStorageChannelDescriptor()
			objsync, _ = CreateObjectStoreSynchronizer(k8sManager.GetConfig(), chdesc, 2)
			objsync.ObjectStore = &utils.FakeObjectStore{}
			Expect(objsync.alginClusterResourceWithHost(tCh)).ShouldNot(HaveOccurred())
			res := &dplv1.Deployable{}
			time.Sleep(time.Second * 5)
			Expect(k8sClient.Get(context.TODO(), types.NamespacedName{Name: dplName, Namespace: tKey.Namespace}, res)).Should(HaveOccurred())
		})
	})

	Context("create deployable on cluster since we get new record(from a user) on bucket", func() {
		var (
			tKey = types.NamespacedName{Name: "tch-2", Namespace: "tns-2"}

			srtName = "srt-ref-2"
			dplName = "t-dpl-2"

			refSrt = &corev1.Secret{
				ObjectMeta: v1.ObjectMeta{
					Name:      srtName,
					Namespace: tKey.Namespace,
				},
				Data: map[string][]byte{
					utils.SecretMapKeyAccessKeyID:     {},
					utils.SecretMapKeySecretAccessKey: {},
				},
			}

			tCh = &chv1.Channel{
				ObjectMeta: v1.ObjectMeta{
					Name:      tKey.Name,
					Namespace: tKey.Namespace,
				},
				Spec: chv1.ChannelSpec{
					Type:     chv1.ChannelType("objectbucket"),
					Pathname: "/" + tKey.Name,
					SecretRef: &corev1.ObjectReference{
						Name:      srtName,
						Namespace: tKey.Namespace,
					},
				},
			}

			expectedConfigMapName = "cm-v2"
			bucketCm              = &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "ConfigMap",
					"apiVersion": "v1",
					"metadata": map[string]interface{}{
						"name":      expectedConfigMapName,
						"namespace": tKey.Namespace,
					},
					"spec": map[string]interface{}{},
				},
			}

			res = &dplv1.Deployable{}

			objsync *ChannelSynchronizer
		)

		It("should grab from bucket and deploy to cluster", func() {
			Expect(k8sClient.Create(context.TODO(), refSrt)).NotTo(HaveOccurred())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), refSrt)).Should(Succeed())
			}()
			Expect(k8sClient.Create(context.TODO(), tCh)).NotTo(HaveOccurred())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), tCh)).Should(Succeed())
			}()

			chdesc, _ := utils.CreateObjectStorageChannelDescriptor()
			objsync, _ = CreateObjectStoreSynchronizer(k8sManager.GetConfig(), chdesc, 2)

			bucketTpl, err := json.Marshal(bucketCm)

			Expect(err).To(BeNil())

			objsync.ObjectStore = &utils.FakeObjectStore{
				Clt: map[string]map[string]utils.DeployableObject{
					tKey.Name: {
						dplName: {
							Name:    dplName,
							Content: bucketTpl,
						},
					},
				},
			}

			Expect(objsync.alginClusterResourceWithHost(tCh)).Should(Succeed())
			time.Sleep(time.Second * 5)

			Expect(k8sClient.Get(context.TODO(), types.NamespacedName{Name: dplName, Namespace: tCh.GetNamespace()}, res)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), res)).Should(Succeed())
			}()
		})

	})

	Context("update deployable on cluster, since the linked resource on bucket updated", func() {
		var (
			tKey = types.NamespacedName{Name: "tch-3", Namespace: "tns-3"}

			srtName = "srt-ref-3"
			refSrt  = &corev1.Secret{
				ObjectMeta: v1.ObjectMeta{
					Name:      srtName,
					Namespace: tKey.Namespace,
				},
				Data: map[string][]byte{
					utils.SecretMapKeyAccessKeyID:     {},
					utils.SecretMapKeySecretAccessKey: {},
				},
			}

			tCh = &chv1.Channel{
				ObjectMeta: v1.ObjectMeta{
					Name:      tKey.Name,
					Namespace: tKey.Namespace,
				},
				Spec: chv1.ChannelSpec{
					Type:     chv1.ChannelType("objectbucket"),
					Pathname: "/" + tKey.Name,
					SecretRef: &corev1.ObjectReference{
						Name:      srtName,
						Namespace: tKey.Namespace,
					},
				},
			}

			dplName = "t-dpl-3"
			tDpl    = &dplv1.Deployable{
				TypeMeta: v1.TypeMeta{
					Kind:       "ConfigMap",
					APIVersion: "apps.open-cluster-management.io/v1",
				},
				ObjectMeta: v1.ObjectMeta{
					Name:      dplName,
					Namespace: tKey.Namespace,
				},
				Spec: dplv1.DeployableSpec{
					Template: &runtime.RawExtension{
						Object: &corev1.ConfigMap{
							TypeMeta: v1.TypeMeta{
								Kind:       "ConfigMap",
								APIVersion: "v1",
							},
							ObjectMeta: v1.ObjectMeta{
								Name: "cm-3",
								Annotations: map[string]string{
									dplv1.AnnotationExternalSource: "true",
								},
							},
						},
					},
				},
			}

			expectedConfigMapName = "cm-v3"
			bucketDpl             = &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "ConfigMap",
					"apiVersion": "v1",
					"metadata": map[string]interface{}{
						"name":      expectedConfigMapName,
						"namespace": tKey.Namespace,
						"annotations": map[string]string{
							dplv1.AnnotationExternalSource: "true",
							dplv1.AnnotationHosting:        tKey.String(),
						},
					},
					"spec": map[string]interface{}{},
				},
			}

			objsync *ChannelSynchronizer
			res     = &dplv1.Deployable{}
		)

		It("should update the deployable on cluster based on the bucket resource", func() {
			Expect(k8sClient.Create(context.TODO(), refSrt)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), refSrt)).Should(Succeed())
			}()
			Expect(k8sClient.Create(context.TODO(), tCh)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), tCh)).Should(Succeed())
			}()

			Expect(k8sClient.Create(context.TODO(), tDpl)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), tDpl)).Should(Succeed())
			}()

			chdesc, _ := utils.CreateObjectStorageChannelDescriptor()

			objsync, _ = CreateObjectStoreSynchronizer(k8sManager.GetConfig(), chdesc, 2)

			eTpl, err := bucketDpl.MarshalJSON()

			Expect(err).Should(BeNil())

			objsync.ObjectStore = &utils.FakeObjectStore{
				Clt: map[string]map[string]utils.DeployableObject{
					tKey.Name: {
						dplName: {
							Name:    dplName,
							Content: eTpl,
						},
					},
				},
			}
			Expect(objsync.alginClusterResourceWithHost(tCh)).Should(Succeed())
			time.Sleep(time.Second * 5)

			Expect(k8sClient.Get(context.TODO(), types.NamespacedName{Name: dplName, Namespace: tKey.Namespace}, res)).ShouldNot(HaveOccurred())

			tpl := &unstructured.Unstructured{}
			Expect(json.Unmarshal(res.Spec.Template.Raw, tpl)).Should(Succeed())
			fmt.Fprintf(GinkgoWriter, "Some log text: %v\n", tpl)

			Expect(tpl.GetName()).To(Equal(expectedConfigMapName))
		})

	})

	Context("delete updated deployable on cluster, since the linked resource on bucket resource is delete", func() {
		var (
			tKey = types.NamespacedName{Name: "tch-3", Namespace: "tns-3"}

			srtName = "srt-ref-3"
			refSrt  = &corev1.Secret{
				ObjectMeta: v1.ObjectMeta{
					Name:      srtName,
					Namespace: tKey.Namespace,
				},
				Data: map[string][]byte{
					utils.SecretMapKeyAccessKeyID:     {},
					utils.SecretMapKeySecretAccessKey: {},
				},
			}

			tCh = &chv1.Channel{
				ObjectMeta: v1.ObjectMeta{
					Name:      tKey.Name,
					Namespace: tKey.Namespace,
				},
				Spec: chv1.ChannelSpec{
					Type:     chv1.ChannelType("objectbucket"),
					Pathname: "/" + tKey.Name,
					SecretRef: &corev1.ObjectReference{
						Name:      srtName,
						Namespace: tKey.Namespace,
					},
				},
			}

			dplName = "t-dpl-3"
			tDpl    = &dplv1.Deployable{
				TypeMeta: v1.TypeMeta{
					Kind:       "ConfigMap",
					APIVersion: "apps.open-cluster-management.io/v1",
				},
				ObjectMeta: v1.ObjectMeta{
					Name:      dplName,
					Namespace: tKey.Namespace,
				},
				Spec: dplv1.DeployableSpec{
					Template: &runtime.RawExtension{
						Object: &corev1.ConfigMap{
							TypeMeta: v1.TypeMeta{
								Kind:       "ConfigMap",
								APIVersion: "v1",
							},
							ObjectMeta: v1.ObjectMeta{
								Name: "cm-3",
								Annotations: map[string]string{
									dplv1.AnnotationExternalSource: "true",
								},
							},
						},
					},
				},
			}

			expectedConfigMapName = "cm-v3"
			bucketDpl             = &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "ConfigMap",
					"apiVersion": "v1",
					"metadata": map[string]interface{}{
						"name":      expectedConfigMapName,
						"namespace": tKey.Namespace,
						"annotations": map[string]string{
							dplv1.AnnotationExternalSource: "true",
							dplv1.AnnotationHosting:        tKey.String(),
						},
					},
					"spec": map[string]interface{}{},
				},
			}

			objsync *ChannelSynchronizer
			res     = &dplv1.Deployable{}
			ctx     = context.TODO()
		)

		It("should delete the deployable on cluster after the resource is delete from bucket", func() {
			Expect(k8sClient.Create(context.TODO(), refSrt)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), refSrt)).Should(Succeed())
			}()

			Expect(k8sClient.Create(context.TODO(), tCh)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), tCh)).Should(Succeed())
			}()
			Expect(k8sClient.Create(context.TODO(), tDpl)).Should(Succeed())

			chdesc, _ := utils.CreateObjectStorageChannelDescriptor()
			objsync, _ = CreateObjectStoreSynchronizer(k8sManager.GetConfig(), chdesc, 2)
			eTpl, err := bucketDpl.MarshalJSON()
			Expect(err).Should(BeNil())

			objsync.ObjectStore = &utils.FakeObjectStore{
				Clt: map[string]map[string]utils.DeployableObject{
					tKey.Name: {
						dplName: {
							Name:    dplName,
							Content: eTpl,
						},
					},
				},
			}
			Expect(objsync.alginClusterResourceWithHost(tCh)).Should(Succeed())
			time.Sleep(time.Second * 5)

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: dplName, Namespace: tKey.Namespace}, res)).ShouldNot(HaveOccurred())

			tpl := &unstructured.Unstructured{}
			Expect(json.Unmarshal(res.Spec.Template.Raw, tpl)).Should(Succeed())

			Expect(tpl.GetName()).To(Equal(expectedConfigMapName))

			objsync.ObjectStore = &utils.FakeObjectStore{}
			Expect(objsync.alginClusterResourceWithHost(tCh)).Should(Succeed())
			time.Sleep(time.Second * 5)

			err = k8sClient.Get(ctx, types.NamespacedName{Name: dplName, Namespace: tKey.Namespace}, &dplv1.Deployable{})
			fmt.Fprintf(GinkgoWriter, "we get error while deleting the deployable, err %v", err)
			Expect(err).ShouldNot(BeNil())
		})
	})
})
