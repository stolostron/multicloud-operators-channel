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

package webhook

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"
)

var _ = Describe("test channel validation logic", func() {
	Context("given an exist namespace channel in a namespace", func() {
		var (
			chkey  = types.NamespacedName{Name: "ch1", Namespace: "default"}
			chnIns = chv1.Channel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      chkey.Name,
					Namespace: chkey.Namespace},
				Spec: chv1.ChannelSpec{
					Type:     chv1.ChannelType(chv1.ChannelTypeNamespace),
					Pathname: chkey.Namespace,
				},
			}
		)

		BeforeEach(func() {
			// Create the Channel object and expect the Reconcile
			Expect(k8sClient.Create(context.TODO(), chnIns.DeepCopy())).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(context.TODO(), &chnIns)).Should(Succeed())
		})

		It("should create git channel", func() {
			dupChn := chnIns.DeepCopy()
			dupChn.Spec.Type = chv1.ChannelTypeGit
			dupChn.SetName("dup-chn1")

			Expect(k8sClient.Create(context.TODO(), dupChn)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), dupChn)).Should(Succeed())
			}()
		})

		It("should not create 2nd  namespace channel", func() {
			dupChn := chnIns.DeepCopy()
			dupChn.SetName("dup-chn1")

			Expect(k8sClient.Create(context.TODO(), dupChn)).ShouldNot(Succeed())
		})

		It("should not create 2nd objectbucket channel", func() {
			dupChn := chnIns.DeepCopy()
			dupChn.Spec.Type = chv1.ChannelTypeObjectBucket
			dupChn.SetName("dup-chn1")

			Expect(k8sClient.Create(context.TODO(), dupChn)).ShouldNot(Succeed())
		})

		It("should not create 2nd  helm channel", func() {
			dupChn := chnIns.DeepCopy()
			dupChn.Spec.Type = chv1.ChannelTypeHelmRepo
			dupChn.SetName("dup-chn1")

			Expect(k8sClient.Create(context.TODO(), dupChn)).ShouldNot(Succeed())
		})
	})

	Context("given exist git channels in a namespace", func() {
		var (
			chkey  = types.NamespacedName{Name: "ch1", Namespace: "default"}
			chnIns = chv1.Channel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      chkey.Name,
					Namespace: chkey.Namespace},
				Spec: chv1.ChannelSpec{
					Type:     chv1.ChannelType(chv1.ChannelTypeGit),
					Pathname: chkey.Namespace,
				},
			}
		)

		BeforeEach(func() {
			// Create the Channel object and expect the Reconcile
			Expect(k8sClient.Create(context.TODO(), chnIns.DeepCopy())).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(context.TODO(), &chnIns)).Should(Succeed())
		})

		It("should create 2nd git channel", func() {
			dupChn := chnIns.DeepCopy()
			dupChn.Spec.Type = chv1.ChannelTypeGit
			dupChn.SetName("dup-chn1")

			Expect(k8sClient.Create(context.TODO(), dupChn)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), dupChn)).Should(Succeed())
			}()
		})

		It("should create 2nd github channel", func() {
			dupChn := chnIns.DeepCopy()
			dupChn.Spec.Type = chv1.ChannelTypeGitHub
			dupChn.SetName("dup-chn1-1")

			Expect(k8sClient.Create(context.TODO(), dupChn)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), dupChn)).Should(Succeed())
			}()
		})

		It("should create 2nd  namespace channel", func() {
			dupChn := chnIns.DeepCopy()
			dupChn.Spec.Type = chv1.ChannelTypeNamespace
			dupChn.SetName("dup-chn1")

			Expect(k8sClient.Create(context.TODO(), dupChn)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), dupChn)).Should(Succeed())
			}()
		})

		It("should create 2nd objectbucket channel", func() {
			dupChn := chnIns.DeepCopy()
			dupChn.Spec.Type = chv1.ChannelTypeObjectBucket
			dupChn.SetName("dup-chn1")

			Expect(k8sClient.Create(context.TODO(), dupChn)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), dupChn)).Should(Succeed())
			}()

		})

		It("should create 2nd  helm channel", func() {
			dupChn := chnIns.DeepCopy()
			dupChn.Spec.Type = chv1.ChannelTypeHelmRepo
			dupChn.SetName("dup-chn1")

			Expect(k8sClient.Create(context.TODO(), dupChn)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), dupChn)).Should(Succeed())
			}()
		})
	})

	Context("given an exist objectbucket channel in a namespace", func() {
		var (
			chkey  = types.NamespacedName{Name: "ch1", Namespace: "default"}
			chnIns = chv1.Channel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      chkey.Name,
					Namespace: chkey.Namespace},
				Spec: chv1.ChannelSpec{
					Type:     chv1.ChannelType(chv1.ChannelTypeObjectBucket),
					Pathname: chkey.Namespace,
				},
			}
		)

		BeforeEach(func() {
			// Create the Channel object and expect the Reconcile
			Expect(k8sClient.Create(context.TODO(), chnIns.DeepCopy())).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(context.TODO(), &chnIns)).Should(Succeed())
		})

		It("should create 2nd git channel", func() {
			dupChn := chnIns.DeepCopy()
			dupChn.Spec.Type = chv1.ChannelTypeGit
			dupChn.SetName("dup-chn1")

			Expect(k8sClient.Create(context.TODO(), dupChn)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), dupChn)).Should(Succeed())
			}()
		})

		It("shouldn't create 2nd  namespace channel", func() {
			dupChn := chnIns.DeepCopy()
			dupChn.Spec.Type = chv1.ChannelTypeNamespace
			dupChn.SetName("dup-chn1")

			Expect(k8sClient.Create(context.TODO(), dupChn)).ShouldNot(Succeed())
		})

		It("shouldn't create 2nd objectbucket channel", func() {
			dupChn := chnIns.DeepCopy()
			dupChn.Spec.Type = chv1.ChannelTypeObjectBucket
			dupChn.SetName("dup-chn1")

			Expect(k8sClient.Create(context.TODO(), dupChn)).ShouldNot(Succeed())
		})

		It("shouldn't create 2nd  helm channel", func() {
			dupChn := chnIns.DeepCopy()
			dupChn.Spec.Type = chv1.ChannelTypeHelmRepo
			dupChn.SetName("dup-chn1")

			Expect(k8sClient.Create(context.TODO(), dupChn)).ShouldNot(Succeed())
		})
	})

	Context("given an exist helm channel in a namespace", func() {
		var (
			chkey  = types.NamespacedName{Name: "ch1", Namespace: "default"}
			chnIns = chv1.Channel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      chkey.Name,
					Namespace: chkey.Namespace},
				Spec: chv1.ChannelSpec{
					Type:     chv1.ChannelType(chv1.ChannelTypeHelmRepo),
					Pathname: chkey.Namespace,
				},
			}
		)

		BeforeEach(func() {
			// Create the Channel object and expect the Reconcile
			Expect(k8sClient.Create(context.TODO(), chnIns.DeepCopy())).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(context.TODO(), &chnIns)).Should(Succeed())
		})

		It("should create 2nd git channel", func() {
			dupChn := chnIns.DeepCopy()
			dupChn.Spec.Type = chv1.ChannelTypeGit
			dupChn.SetName("dup-chn1")

			Expect(k8sClient.Create(context.TODO(), dupChn)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), dupChn)).Should(Succeed())
			}()
		})

		It("shouldn't create 2nd  namespace channel", func() {
			dupChn := chnIns.DeepCopy()
			dupChn.Spec.Type = chv1.ChannelTypeNamespace
			dupChn.SetName("dup-chn1")

			Expect(k8sClient.Create(context.TODO(), dupChn)).ShouldNot(Succeed())
		})

		It("shouldn't create 2nd objectbucket channel", func() {
			dupChn := chnIns.DeepCopy()
			dupChn.Spec.Type = chv1.ChannelTypeObjectBucket
			dupChn.SetName("dup-chn1")

			Expect(k8sClient.Create(context.TODO(), dupChn)).ShouldNot(Succeed())
		})

		It("shouldn't create 2nd  helm channel", func() {
			dupChn := chnIns.DeepCopy()
			dupChn.Spec.Type = chv1.ChannelTypeHelmRepo
			dupChn.SetName("dup-chn1")

			Expect(k8sClient.Create(context.TODO(), dupChn)).ShouldNot(Succeed())
		})
	})

})
