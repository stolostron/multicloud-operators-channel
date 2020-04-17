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

package deployable

import (
	"time"

	tlog "github.com/go-logr/logr/testing"
	"golang.org/x/net/context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"
	dplv1 "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis/apps/v1"
)

const (
	targetNamespace      = "default"
	targetChannelName    = "channel-deployable-reconcile"
	targetChannelType    = chv1.ChannelType("namespace")
	targetDeployableName = "t-deployable"
	k8swait              = time.Second * 2
	GCWait               = time.Second * 5
	fakeRecordBufferSize = 8
)

var _ = Describe("channel and deployable requeue", func() {
	var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: targetDeployableName, Namespace: targetNamespace}}
	It("given channel without deployable, should not get reconcile signal", func() {
		channelInstance := chv1.Channel{
			ObjectMeta: metav1.ObjectMeta{
				Name:      targetChannelName,
				Namespace: targetNamespace},
			Spec: chv1.ChannelSpec{
				Type:     targetChannelType,
				Pathname: targetNamespace,
			},
		}

		recFn, requests := SetupTestReconcile(newReconciler(k8sManager, record.NewFakeRecorder(fakeRecordBufferSize), tlog.NullLogger{}))
		Expect(add(k8sManager, recFn, tlog.NullLogger{})).NotTo(HaveOccurred())

		// Create the Channel object and expect the Reconcile
		Expect(k8sClient.Create(context.TODO(), channelInstance.DeepCopy())).NotTo(HaveOccurred())

		defer func() {
			Expect(k8sClient.Delete(context.TODO(), &channelInstance)).Should(Succeed())
		}()

		time.Sleep(k8swait)
		// if there's no deployables, then we won't have any reconcile requests
		Eventually(requests, k8swait).ShouldNot(Receive(Equal(expectedRequest)))

	})

	It("should generate reconcile signal when creating deployable resource", func() {
		deployableInstance := dplv1.Deployable{
			ObjectMeta: metav1.ObjectMeta{
				Name:      targetDeployableName,
				Namespace: targetNamespace,
			},
			Spec: dplv1.DeployableSpec{
				Template: &runtime.RawExtension{
					Object: &corev1.ConfigMap{},
				},
			},
		}
		recFn, requests := SetupTestReconcile(newReconciler(k8sManager, record.NewFakeRecorder(fakeRecordBufferSize), tlog.NullLogger{}))
		Expect(add(k8sManager, recFn, tlog.NullLogger{})).NotTo(HaveOccurred())

		Expect(k8sClient.Create(context.TODO(), deployableInstance.DeepCopy())).NotTo(HaveOccurred())

		defer func() {
			Expect(k8sClient.Delete(context.TODO(), &deployableInstance)).Should(Succeed())
		}()
		time.Sleep(k8swait)

		//if there's deployables, then the channel update will generate requests on all the existing deployables
		Eventually(requests, k8swait).Should(Receive(Equal(expectedRequest)))
	})
})

var _ = Describe("promote deployables to channel namespace without considering channel gate", func() {
	var (
		dplKey = types.NamespacedName{
			Name:      targetDeployableName,
			Namespace: "dpl-ns-2",
		}

		chKey = types.NamespacedName{
			Name:      targetChannelName,
			Namespace: "ch-ns-2",
		}

		channelInstance = chv1.Channel{
			ObjectMeta: metav1.ObjectMeta{
				Name:      chKey.Name,
				Namespace: chKey.Namespace},
			Spec: chv1.ChannelSpec{
				Type:     targetChannelType,
				Pathname: chKey.Namespace,
			},
		}
		deployableInstance = dplv1.Deployable{
			ObjectMeta: metav1.ObjectMeta{
				Name:      dplKey.Name,
				Namespace: dplKey.Namespace,
			},
			Spec: dplv1.DeployableSpec{
				Template: &runtime.RawExtension{
					Object: &corev1.ConfigMap{},
				},
			},
		}
	)
	BeforeEach(func() {
	})
	AfterEach(func() {
	})

	PContext("given a channel with sourceNamespaces without gate", func() {
		It("channel without sourceNamespaces, deployable have target channel", func() {
			chn := channelInstance.DeepCopy()
			Expect(k8sClient.Create(context.TODO(), chn)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), chn)).Should(Succeed())
			}()

			time.Sleep(k8swait)
			recFn := newReconciler(k8sManager, record.NewFakeRecorder(fakeRecordBufferSize), tlog.NullLogger{})
			Expect(add(k8sManager, recFn, tlog.NullLogger{})).NotTo(HaveOccurred())

			dpl := deployableInstance.DeepCopy()
			dpl.Spec.Channels = []string{chKey.Name}
			Expect(k8sClient.Create(context.TODO(), dpl)).NotTo(HaveOccurred())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), dpl)).Should(Succeed())
			}()

			time.Sleep(k8swait)
			req := reconcile.Request{NamespacedName: dplKey}
			recFn.Reconcile(req)

			expectDpls := dplv1.DeployableList{}
			Expect(k8sClient.List(context.TODO(), &expectDpls,
				&ctrl.ListOptions{
					Namespace: chKey.Namespace,
				})).Should(Succeed())
			Expect(expectDpls.Items).Should(HaveLen(1))

			for _, item := range expectDpls.Items {
				Expect(k8sClient.Delete(context.TODO(), &item)).Should(Succeed())
			}

		})

		It("given channel without gate and deployable spec points to different channel", func() {
			chn := channelInstance.DeepCopy()
			Expect(k8sClient.Create(context.TODO(), chn)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), chn)).Should(Succeed())
			}()
			dpl := deployableInstance.DeepCopy()
			dpl.Spec.Channels = []string{"wrong"}

			recFn := newReconciler(k8sManager, record.NewFakeRecorder(fakeRecordBufferSize), tlog.NullLogger{})
			Expect(add(k8sManager, recFn, tlog.NullLogger{})).NotTo(HaveOccurred())

			Expect(k8sClient.Create(context.TODO(), dpl)).NotTo(HaveOccurred())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), dpl)).Should(Succeed())
			}()

			time.Sleep(k8swait)
			req := reconcile.Request{NamespacedName: dplKey}
			recFn.Reconcile(req)

			expectDpls := dplv1.DeployableList{}
			Expect(k8sClient.List(context.TODO(), &expectDpls,
				&ctrl.ListOptions{
					Namespace: chKey.Namespace,
				})).Should(Succeed())
			Expect(expectDpls.Items).Should(HaveLen(0))
			for _, item := range expectDpls.Items {
				Expect(k8sClient.Delete(context.TODO(), &item)).Should(Succeed())
			}
		})

		It("given channel without gate and deployable with channel at spec", func() {
			chn := channelInstance.DeepCopy()
			chn.Spec.SourceNamespaces = []string{dplKey.Namespace}

			Expect(k8sClient.Create(context.TODO(), chn)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), chn)).Should(Succeed())
			}()

			time.Sleep(k8swait)
			recFn := newReconciler(k8sManager, record.NewFakeRecorder(fakeRecordBufferSize), tlog.NullLogger{})
			Expect(add(k8sManager, recFn, tlog.NullLogger{})).NotTo(HaveOccurred())

			dpl := deployableInstance.DeepCopy()
			Expect(k8sClient.Create(context.TODO(), dpl)).NotTo(HaveOccurred())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), dpl)).Should(Succeed())
			}()

			time.Sleep(k8swait)
			req := reconcile.Request{NamespacedName: dplKey}
			recFn.Reconcile(req)

			expectDpls := dplv1.DeployableList{}
			Expect(k8sClient.List(context.TODO(), &expectDpls,
				&ctrl.ListOptions{
					Namespace: chKey.Namespace,
				})).Should(Succeed())

			// TODO, should having len 1...
			Expect(expectDpls.Items).Should(HaveLen(0))

			for _, item := range expectDpls.Items {
				Expect(k8sClient.Delete(context.TODO(), &item)).Should(Succeed())
			}
		})

		It("given channel without gate and deployable spec points to different channel", func() {
			chn := channelInstance.DeepCopy()
			chn.Spec.SourceNamespaces = []string{dplKey.Namespace}
			Expect(k8sClient.Create(context.TODO(), chn)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), chn)).Should(Succeed())
			}()

			dpl := deployableInstance.DeepCopy()
			dpl.Spec.Channels = []string{"wrong"}

			recFn := newReconciler(k8sManager, record.NewFakeRecorder(fakeRecordBufferSize), tlog.NullLogger{})
			Expect(add(k8sManager, recFn, tlog.NullLogger{})).NotTo(HaveOccurred())

			Expect(k8sClient.Create(context.TODO(), dpl)).NotTo(HaveOccurred())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), dpl)).Should(Succeed())
			}()

			time.Sleep(k8swait)
			req := reconcile.Request{NamespacedName: dplKey}
			recFn.Reconcile(req)

			expectDpls := dplv1.DeployableList{}
			Expect(k8sClient.List(context.TODO(), &expectDpls,
				&ctrl.ListOptions{
					Namespace: chKey.Namespace,
				})).Should(Succeed())
			Expect(expectDpls.Items).Should(HaveLen(0))
			for _, item := range expectDpls.Items {
				Expect(k8sClient.Delete(context.TODO(), &item)).Should(Succeed())
			}
		})
	})

	Context("given a channel with sourceNamespaces with gate", func() {
		It("given channel without gate and deployable with channel at spec", func() {
			chn := channelInstance.DeepCopy()
			chn.Spec.SourceNamespaces = []string{dplKey.Namespace}
			chn.Spec.Gates = &chv1.ChannelGate{Annotations: map[string]string{"test": "pass"}}

			Expect(k8sClient.Create(context.TODO(), chn)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), chn)).Should(Succeed())
			}()

			recFn := newReconciler(k8sManager, record.NewFakeRecorder(fakeRecordBufferSize), tlog.NullLogger{})
			Expect(add(k8sManager, recFn, tlog.NullLogger{})).NotTo(HaveOccurred())

			dpl := deployableInstance.DeepCopy()
			dpl.Annotations = map[string]string{"test": "pass"}
			Expect(k8sClient.Create(context.TODO(), dpl)).NotTo(HaveOccurred())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), dpl)).Should(Succeed())
			}()

			time.Sleep(k8swait)
			req := reconcile.Request{NamespacedName: dplKey}
			recFn.Reconcile(req)

			expectDpls := dplv1.DeployableList{}
			Expect(k8sClient.List(context.TODO(), &expectDpls,
				&ctrl.ListOptions{
					Namespace: chKey.Namespace,
				})).Should(Succeed())
			// TODO
			Expect(expectDpls.DeepCopy().Items).Should(HaveLen(1))
			for _, item := range expectDpls.Items {
				Expect(k8sClient.Delete(context.TODO(), &item)).Should(Succeed())
			}
		})

		It("given channel without gate and deployable spec points to different channel", func() {
			chn := channelInstance.DeepCopy()
			chn.Spec.SourceNamespaces = []string{dplKey.Namespace}
			chn.Spec.Gates = &chv1.ChannelGate{Annotations: map[string]string{"test": "pass"}}
			Expect(k8sClient.Create(context.TODO(), chn)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), chn)).Should(Succeed())
			}()

			dpl := deployableInstance.DeepCopy()
			dpl.Annotations = map[string]string{"test": "not-pass"}

			recFn := newReconciler(k8sManager, record.NewFakeRecorder(fakeRecordBufferSize), tlog.NullLogger{})
			Expect(add(k8sManager, recFn, tlog.NullLogger{})).NotTo(HaveOccurred())

			Expect(k8sClient.Create(context.TODO(), dpl)).NotTo(HaveOccurred())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), dpl)).Should(Succeed())
			}()

			time.Sleep(k8swait)
			req := reconcile.Request{NamespacedName: dplKey}
			recFn.Reconcile(req)

			expectDpls := dplv1.DeployableList{}
			Expect(k8sClient.List(context.TODO(), &expectDpls,
				&ctrl.ListOptions{
					Namespace: chKey.Namespace,
				})).Should(Succeed())
			Expect(expectDpls.Items).Should(HaveLen(0))
			for _, item := range expectDpls.Items {
				Expect(k8sClient.Delete(context.TODO(), &item)).Should(Succeed())
			}
		})
	})

	Context("given a channel with sourceNamespaces with gate", func() {
		It("given channel with gate and deployable with matched annotation should be promoted and GC'ed properly", func() {
			time.Sleep(k8swait)

			chn := channelInstance.DeepCopy()
			chn.Spec.SourceNamespaces = []string{dplKey.Namespace}
			chn.Spec.Gates = &chv1.ChannelGate{Annotations: map[string]string{"test": "pass"}}
			Expect(k8sClient.Create(context.TODO(), chn)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), chn)).Should(Succeed())
			}()

			time.Sleep(k8swait)
			recFn := newReconciler(k8sManager, record.NewFakeRecorder(fakeRecordBufferSize), tlog.NullLogger{})
			Expect(add(k8sManager, recFn, tlog.NullLogger{})).NotTo(HaveOccurred())

			dpl := deployableInstance.DeepCopy()
			dpl.Annotations = map[string]string{"test": "pass"}
			Expect(k8sClient.Create(context.TODO(), dpl)).NotTo(HaveOccurred())

			time.Sleep(k8swait)
			req := reconcile.Request{NamespacedName: dplKey}
			recFn.Reconcile(req)

			expectDpls := dplv1.DeployableList{}
			Expect(k8sClient.List(context.TODO(), &expectDpls,
				&ctrl.ListOptions{
					Namespace: chKey.Namespace,
				})).Should(Succeed())
			Expect(expectDpls.Items).Should(HaveLen(1))

			Expect(k8sClient.Delete(context.TODO(), dpl)).Should(Succeed())

			Expect(expectDpls.Items[0].GetOwnerReferences()[0].Name).Should(Equal(dpl.Name))

			for _, item := range expectDpls.Items {
				Expect(k8sClient.Delete(context.TODO(), &item)).Should(Succeed())
			}
		})
	})
})
