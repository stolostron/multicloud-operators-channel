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

package github

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"
	gitsync "github.com/open-cluster-management/multicloud-operators-channel/pkg/synchronizer/githubsynchronizer"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("reconcile github channel", func() {

	const (
		timeout  = time.Second * 5
		interval = 3
	)

	var (
		chn   *chv1.Channel
		chkey types.NamespacedName
		ctx   = context.TODO()
		gsync *gitsync.ChannelSynchronizer
		rec   reconcile.Reconciler
		err   error
	)

	BeforeEach(func() {
		// Add any setup steps that needs to be executed before each test
		chname, chns := "t-ch", "t-ns"
		chkey = types.NamespacedName{Name: chname, Namespace: chns}
		chn = &chv1.Channel{
			ObjectMeta: v1.ObjectMeta{
				Name:      chname,
				Namespace: chns,
			},
			Spec: chv1.ChannelSpec{
				Type:     chv1.ChannelType("namespace"),
				Pathname: "",
			},
		}
	})

	AfterEach(func() {
		// Add any teardown steps that needs to be executed after each test
	})

	Context("Wrong Channel tpye", func() {
		BeforeEach(func() {
			gsync, err = gitsync.CreateGithubSynchronizer(k8sManager.GetConfig(), k8sManager.GetScheme(), interval)
			Expect(err).ShouldNot(HaveOccurred())
			rec = newReconciler(k8sManager, gsync)
		})

		It("deletion reconcile", func() {
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      chn.Name,
					Namespace: chn.Namespace,
				},
			}

			_, err = rec.Reconcile(req)
			Expect(err).NotTo(HaveOccurred())
		})

		It("create or update", func() {
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      chn.Name,
					Namespace: chn.Namespace,
				},
			}

			Expect(k8sClient.Create(ctx, chn)).Should(Succeed())
			defer k8sClient.Delete(ctx, chn)

			_, err = rec.Reconcile(req)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Github Channel tpye", func() {
		BeforeEach(func() {
			gsync, err = gitsync.CreateGithubSynchronizer(k8sManager.GetConfig(), k8sManager.GetScheme(), interval)
			chn.Spec.Type = chv1.ChannelType("github")
			Expect(err).ShouldNot(HaveOccurred())
			rec = newReconciler(k8sManager, gsync)
		})

		It("deletion reconcile with empty channel registry", func() {
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      chn.Name,
					Namespace: chn.Namespace,
				},
			}

			_, err = rec.Reconcile(req)
			Expect(err).NotTo(HaveOccurred())
		})

		It("deletion reconcile with channel registry", func() {
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      chn.Name,
					Namespace: chn.Namespace,
				},
			}

			gsync.ChannelMap[chkey] = chn
			rec = newReconciler(k8sManager, gsync)
			_, err = rec.Reconcile(req)
			Expect(err).NotTo(HaveOccurred())

			Expect(gsync.ChannelMap[chkey]).Should(BeNil())
		})

		It("create or update", func() {
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      chn.Name,
					Namespace: chn.Namespace,
				},
			}

			Expect(k8sClient.Create(ctx, chn)).Should(Succeed())
			defer k8sClient.Delete(ctx, chn)

			_, err = rec.Reconcile(req)
			Expect(err).NotTo(HaveOccurred())

			Expect(gsync.ChannelMap[chkey]).NotTo(BeNil())
		})
	})
})
