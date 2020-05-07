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

package objectstore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	tlog "github.com/go-logr/logr/testing"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"
	"github.com/open-cluster-management/multicloud-operators-channel/pkg/utils"
	dplv1 "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis/apps/v1"
)

const (
	timeout = time.Second * 2
	k8swait = time.Second * 5
)

var _ = Describe("object bucket controller", func() {
	BeforeEach(func() {
	})

	AfterEach(func() {
	})

	Context("detect any deployable reconcile signal", func() {
		var (
			testNamespace = "a-ns"
			testDplName   = "tdpl"
			theAnno       = map[string]string{"who": "bear"}

			// this is trvial, since we are only to test if the controller is
			// singal on the given deployable
			expectedResult = reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: testNamespace,
					Name:      testDplName,
				},
			}
			dpl = &dplv1.Deployable{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testDplName,
					Namespace:   testNamespace,
					Annotations: theAnno,
				},
				Spec: dplv1.DeployableSpec{
					Template: &runtime.RawExtension{
						Object: &v1.ConfigMap{},
					},
				},
			}
			ctx = context.TODO()
		)

		BeforeEach(func() {

		})

		It("should reconcile on creation of a deployable", func() {

			chRegistry, err := utils.CreateObjectStorageChannelDescriptor()
			Expect(err).NotTo(HaveOccurred())

			rec := newReconciler(k8sManager, chRegistry, tlog.NullLogger{})
			recFn, result := SetupTestReconcile(rec)

			Expect(add(k8sManager, recFn, tlog.NullLogger{})).NotTo(HaveOccurred())

			Expect(k8sClient.Create(ctx, dpl)).NotTo(HaveOccurred())

			defer func() {
				Expect(k8sClient.Delete(ctx, dpl)).Should(Succeed())
			}()

			Eventually(result, timeout).Should(Receive(Equal(expectedResult)))

		})
	})

	Context("get channel info from channel registry", func() {
		var (
			testNamespace = "a-ns"
			testCh        = "obj"
			ctx           = context.TODO()
		)

		type testCase struct {
			desc   string
			tRec   *ReconcileDeployable
			dfChan *chv1.Channel
			tName  string
			wantCh *chv1.Channel
		}

		It("should return nil, since channel registry is empty", func() {
			tC := testCase{
				desc: "none channel at hub",
				tRec: &ReconcileDeployable{
					KubeClient:        k8sClient,
					ChannelDescriptor: &utils.ChannelDescriptor{},
				},
				tName:  testNamespace,
				wantCh: nil,
			}
			gotCh := tC.tRec.getChannelForNamespace(testNamespace, tlog.NullLogger{})
			Expect(assertChannelObject(tC.wantCh, gotCh)).To(BeNil())
		})

		It("should return nil, querying channel doesn't have a type", func() {
			tC := testCase{
				desc: "channel without spec type at hub",
				tRec: &ReconcileDeployable{
					KubeClient:        k8sClient,
					ChannelDescriptor: &utils.ChannelDescriptor{},
				},
				dfChan: &chv1.Channel{
					ObjectMeta: metav1.ObjectMeta{
						Name:        testCh,
						Namespace:   testNamespace,
						Annotations: map[string]string{},
					},
					Spec: chv1.ChannelSpec{
						Type: chv1.ChannelTypeGit,
					},
				},
				tName:  testNamespace,
				wantCh: nil,
			}
			Expect(tC.tRec.KubeClient.Create(ctx, tC.dfChan)).Should(Succeed())
			defer func() {
				Expect(tC.tRec.KubeClient.Delete(ctx, tC.dfChan)).Should(Succeed())
			}()
			time.Sleep(k8swait)
			gotCh := tC.tRec.getChannelForNamespace(testNamespace, tlog.NullLogger{})
			fmt.Fprintf(GinkgoWriter, "%v", gotCh)
			Expect(assertChannelObject(tC.wantCh, gotCh)).To(BeNil())
		})

		It("should return a valid channel ", func() {
			tC := testCase{
				desc: "channel with spec type at hub",
				tRec: &ReconcileDeployable{
					KubeClient:        k8sClient,
					ChannelDescriptor: &utils.ChannelDescriptor{},
				},
				dfChan: &chv1.Channel{
					ObjectMeta: metav1.ObjectMeta{
						Name:        testCh,
						Namespace:   testNamespace,
						Annotations: map[string]string{},
					},
					Spec: chv1.ChannelSpec{
						Type: chv1.ChannelTypeObjectBucket,
					},
				},
				tName: testNamespace,
				wantCh: &chv1.Channel{
					ObjectMeta: metav1.ObjectMeta{
						Name:        testCh,
						Namespace:   testNamespace,
						Annotations: map[string]string{},
					},
					Spec: chv1.ChannelSpec{
						Type: chv1.ChannelTypeObjectBucket,
					},
				},
			}
			Expect(tC.tRec.KubeClient.Create(ctx, tC.dfChan)).Should(Succeed())
			defer func() {
				Expect(tC.tRec.KubeClient.Delete(ctx, tC.dfChan)).Should(Succeed())
			}()
			time.Sleep(k8swait)
			gotCh := tC.tRec.getChannelForNamespace(testNamespace, tlog.NullLogger{})
			Expect(assertChannelObject(tC.wantCh, gotCh)).To(BeNil())
		})

		It("should return nil", func() {
			tC := testCase{
				desc: "channel with spec type at hub",
				tRec: &ReconcileDeployable{
					KubeClient:        k8sClient,
					ChannelDescriptor: &utils.ChannelDescriptor{},
				},
				dfChan: &chv1.Channel{
					ObjectMeta: metav1.ObjectMeta{
						Name:        testCh,
						Namespace:   testNamespace,
						Annotations: map[string]string{},
					},
					Spec: chv1.ChannelSpec{
						Type: chv1.ChannelTypeGit,
					},
				},
				tName:  testNamespace,
				wantCh: nil,
			}

			Expect(tC.tRec.KubeClient.Create(ctx, tC.dfChan)).Should(Succeed())
			defer func() {
				Expect(tC.tRec.KubeClient.Delete(ctx, tC.dfChan)).Should(Succeed())
			}()
			time.Sleep(k8swait)
			gotCh := tC.tRec.getChannelForNamespace(testNamespace, tlog.NullLogger{})
			Expect(assertChannelObject(tC.wantCh, gotCh)).To(BeNil())
		})
	})

	Context("with fake client to see the push to bucket logic", func() {
		var (
			testNamespace = "a-ns"
			testDplName   = "tdpl-1"
			theAnno       = map[string]string{"who": "bear"}
			chname        = "my-objectstore-chn"
			bucket        = "fake"

			chn = &chv1.Channel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      chname,
					Namespace: testNamespace,
				},
				Spec: chv1.ChannelSpec{
					Type:     chv1.ChannelType("ObjectBucket"),
					Pathname: "/" + bucket,
				},
			}

			refCmName = "ch-cm"
			refCm     = v1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					Kind: "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      refCmName,
					Namespace: testNamespace,
				},
			}
			ctx = context.TODO()
		)

		It("should generate object at the fake client", func() {
			chReg, err := utils.CreateObjectStorageChannelDescriptor()
			Expect(err).NotTo(HaveOccurred())

			myStoreage := &utils.FakeObjectStore{}
			myStoreage.InitObjectStoreConnection("", "", "")

			chReg.SetObjectStorageForChannel(chn, myStoreage)

			cmRaw, _ := json.Marshal(refCm)

			dpl := &dplv1.Deployable{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testDplName,
					Namespace:   testNamespace,
					Annotations: theAnno,
				},
				Spec: dplv1.DeployableSpec{
					Template: &runtime.RawExtension{Raw: cmRaw},
					Channels: []string{chname},
				},
			}

			defer func() {
				Expect(k8sClient.Delete(ctx, dpl)).Should(Succeed())
			}()
			Expect(k8sClient.Create(ctx, dpl)).Should(Succeed())

			defer func() {
				Expect(k8sClient.Delete(ctx, chn)).Should(Succeed())
			}()
			Expect(k8sClient.Create(ctx, chn)).Should(Succeed())

			time.Sleep(k8swait)

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      dpl.Name,
					Namespace: dpl.Namespace,
				},
			}

			rec := newReconciler(k8sManager, chReg, tlog.NullLogger{})

			_, err = rec.Reconcile(req)
			Expect(err).Should(BeNil())

			_, err = myStoreage.Get(bucket, dpl.Name)
			Expect(err).Should(BeNil())
		})
	})

})

func assertChannelObject(want, got *chv1.Channel) error {
	if want == nil && got == nil {
		return nil
	}

	if want == nil && got != nil {
		return fmt.Errorf("got nil")
	}

	if want != nil && got == nil {
		return fmt.Errorf("wanted %v got %v", want, got)
	}

	if want.GetName() == got.GetName() && want.GetNamespace() == got.GetNamespace() {
		return nil
	}

	return fmt.Errorf("got wrong channel")
}
