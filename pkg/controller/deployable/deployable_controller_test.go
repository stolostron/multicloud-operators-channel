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
	"fmt"
	"time"

	"golang.org/x/net/context"

	tlog "github.com/go-logr/logr/testing"
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	mgr "sigs.k8s.io/controller-runtime/pkg/manager"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"
	"github.com/open-cluster-management/multicloud-operators-channel/pkg/utils"
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
	var (
		expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: targetDeployableName, Namespace: targetNamespace}}
	)

	var stop chan struct{}
	var k8sClient client.Client

	BeforeEach(func() {
		stop = make(chan struct{})

		k8sManager, err := mgr.New(cCfg, mgr.Options{MetricsBindAddress: "0"})
		Expect(err).ToNot(HaveOccurred())

		recFn, requests = SetupTestReconcile(newReconciler(k8sManager, record.NewFakeRecorder(fakeRecordBufferSize), tlog.NullLogger{}))
		Expect(add(k8sManager, recFn, tlog.NullLogger{})).NotTo(HaveOccurred())

		go func() {
			Expect(k8sManager.Start(stop)).ToNot(HaveOccurred())
		}()

		k8sClient = k8sManager.GetClient()
		Expect(k8sClient).ToNot(BeNil())

	})

	AfterEach(func() {
		close(stop)
	})

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

	var stop chan struct{}
	var k8sClient client.Client

	BeforeEach(func() {
		stop = make(chan struct{})

		k8sManager, err := mgr.New(cCfg, mgr.Options{MetricsBindAddress: "0"})
		Expect(err).ToNot(HaveOccurred())

		recFn, requests = SetupTestReconcile(newReconciler(k8sManager, record.NewFakeRecorder(fakeRecordBufferSize), tlog.NullLogger{}))
		Expect(add(k8sManager, recFn, tlog.NullLogger{})).NotTo(HaveOccurred())

		go func() {
			Expect(k8sManager.Start(stop)).ToNot(HaveOccurred())
		}()

		k8sClient = k8sManager.GetClient()
		Expect(k8sClient).ToNot(BeNil())

	})

	AfterEach(func() {
		close(stop)
	})

	Context("channel points to deployable or deployable points to channel", func() {
		It("should promote deployable to channel, since deployable points to a no gate channel", func() {
			chn := channelInstance.DeepCopy()
			randStr := fmt.Sprintf("-%v", rand.Intn(100))

			chn.SetName(chn.GetName() + randStr)
			Expect(k8sClient.Create(context.TODO(), chn)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), chn)).Should(Succeed())
			}()

			time.Sleep(k8swait)

			dpl := deployableInstance.DeepCopy()
			dpl.SetName(dpl.GetName() + randStr)
			dpl.Spec.Channels = []string{chn.GetName()}
			Expect(k8sClient.Create(context.TODO(), dpl)).NotTo(HaveOccurred())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), dpl)).Should(Succeed())
			}()

			time.Sleep(k8swait)

			expectDpls := dplv1.DeployableList{}
			Expect(k8sClient.List(context.TODO(), &expectDpls, client.InNamespace(chn.GetNamespace()))).Should(Succeed())

			Expect(expectDpls.Items).ShouldNot(HaveLen(0))

			for _, item := range expectDpls.Items {
				Expect(k8sClient.Delete(context.TODO(), &item)).Should(Succeed())
			}
		})

		It("should not promote deployable to channel, since deployable is not pointing to wrong channel and channel not looking at deployable's namespace]", func() {
			chn := channelInstance.DeepCopy()

			randStr := fmt.Sprintf("-%v", rand.Intn(100))
			chn.SetName(chn.GetName() + randStr)

			Expect(k8sClient.Create(context.TODO(), chn)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), chn)).Should(Succeed())
			}()

			dpl := deployableInstance.DeepCopy()
			dpl.SetName(dpl.GetName() + randStr)
			dpl.Spec.Channels = []string{"wrong"}

			Expect(k8sClient.Create(context.TODO(), dpl)).NotTo(HaveOccurred())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), dpl)).Should(Succeed())
			}()

			time.Sleep(k8swait)

			expectDpls := dplv1.DeployableList{}
			Expect(k8sClient.List(context.TODO(), &expectDpls, client.InNamespace(chn.GetNamespace()))).Should(Succeed())

			Expect(expectDpls.Items).Should(HaveLen(0))
			for _, item := range expectDpls.Items {
				Expect(k8sClient.Delete(context.TODO(), &item)).Should(Succeed())
			}
		})

		It("should NOT promote deployable to channel since channel don't have gate even active watch the deployable's namespace", func() {
			chn := channelInstance.DeepCopy()
			randStr := fmt.Sprintf("-%v", rand.Intn(100))

			chn.SetName(chn.GetName() + randStr)
			chn.Spec.SourceNamespaces = []string{dplKey.Namespace}

			Expect(k8sClient.Create(context.TODO(), chn)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), chn)).Should(Succeed())
			}()

			time.Sleep(k8swait)

			dpl := deployableInstance.DeepCopy()
			dpl.SetName(dpl.GetName() + randStr)
			Expect(k8sClient.Create(context.TODO(), dpl)).NotTo(HaveOccurred())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), dpl)).Should(Succeed())
			}()

			time.Sleep(k8swait)

			expectDpls := dplv1.DeployableList{}
			Expect(k8sClient.List(context.TODO(), &expectDpls, client.InNamespace(chn.GetNamespace()))).Should(Succeed())

			Expect(expectDpls.Items).Should(HaveLen(0))

			for _, item := range expectDpls.Items {
				Expect(k8sClient.Delete(context.TODO(), &item)).Should(Succeed())
			}
		})
	})

	Context("given a channel with sourceNamespaces with gate", func() {
		It("should promote deployable to channel's namespace", func() {
			chn := channelInstance.DeepCopy()
			chn.Spec.SourceNamespaces = []string{dplKey.Namespace}
			chn.Spec.Gates = &chv1.ChannelGate{Annotations: map[string]string{"test": "pass"}}

			randStr := fmt.Sprintf("-%v", rand.Intn(100))
			chn.SetName(chn.GetName() + randStr)

			Expect(k8sClient.Create(context.TODO(), chn)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), chn)).Should(Succeed())
			}()

			time.Sleep(k8swait)

			dpl := deployableInstance.DeepCopy()
			dpl.SetName(dpl.GetName() + randStr)
			dpl.Annotations = map[string]string{"test": "pass"}
			Expect(k8sClient.Create(context.TODO(), dpl)).NotTo(HaveOccurred())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), dpl)).Should(Succeed())
			}()

			time.Sleep(k8swait)

			expectDpls := dplv1.DeployableList{}
			Expect(k8sClient.List(context.TODO(), &expectDpls, client.InNamespace(chn.GetNamespace()))).Should(Succeed())
			Expect(expectDpls.DeepCopy().Items).ShouldNot(HaveLen(0))

			for _, item := range expectDpls.Items {
				Expect(k8sClient.Delete(context.TODO(), &item)).Should(Succeed())
			}
		})

		It("should not promote deployable due to mismatch annotation gate", func() {
			chn := channelInstance.DeepCopy()
			chn.Spec.SourceNamespaces = []string{dplKey.Namespace}
			chn.Spec.Gates = &chv1.ChannelGate{Annotations: map[string]string{"test": "pass"}}

			randStr := fmt.Sprintf("-%v", rand.Intn(100))
			chn.SetName(chn.GetName() + randStr)
			Expect(k8sClient.Create(context.TODO(), chn)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), chn)).Should(Succeed())
			}()

			time.Sleep(k8swait)

			dpl := deployableInstance.DeepCopy()
			dpl.SetName(dpl.GetName() + randStr)
			dpl.Annotations = map[string]string{"test": "not-pass"}

			Expect(k8sClient.Create(context.TODO(), dpl)).NotTo(HaveOccurred())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), dpl)).Should(Succeed())
			}()

			time.Sleep(k8swait)

			expectDpls := dplv1.DeployableList{}
			Expect(k8sClient.List(context.TODO(), &expectDpls, client.InNamespace(chn.GetNamespace()))).Should(Succeed())
			Expect(expectDpls.Items).Should(HaveLen(0))
			for _, item := range expectDpls.Items {
				Expect(k8sClient.Delete(context.TODO(), &item)).Should(Succeed())
			}
		})
	})

	Context("given a channel with sourceNamespaces with gate", func() {
		It("should promote deployable and GC'ed properly after the original deployable is deleted", func() {
			time.Sleep(k8swait)

			chn := channelInstance.DeepCopy()
			chn.Spec.SourceNamespaces = []string{dplKey.Namespace}
			chn.Spec.Gates = &chv1.ChannelGate{Annotations: map[string]string{"test": "pass"}}
			randStr := fmt.Sprintf("-%v", rand.Intn(100))
			chn.SetName(chn.GetName() + randStr)

			Expect(k8sClient.Create(context.TODO(), chn)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(context.TODO(), chn)).Should(Succeed())
			}()

			time.Sleep(k8swait)

			dpl := deployableInstance.DeepCopy()
			dpl.Annotations = map[string]string{"test": "pass"}
			dpl.SetName(dpl.GetName() + randStr)
			Expect(k8sClient.Create(context.TODO(), dpl)).NotTo(HaveOccurred())

			time.Sleep(k8swait)

			expectDpls := dplv1.DeployableList{}
			Expect(k8sClient.List(context.TODO(), &expectDpls, client.InNamespace(chn.GetNamespace()))).Should(Succeed())

			Expect(expectDpls.Items).ShouldNot(HaveLen(0))

			Expect(k8sClient.Delete(context.TODO(), dpl)).Should(Succeed())

			Expect(expectDpls.Items[0].GetOwnerReferences()[0].Name).Should(Equal(dpl.Name))

			for _, item := range expectDpls.Items {
				Expect(k8sClient.Delete(context.TODO(), &item)).Should(Succeed())
			}
		})
	})
})

var _ = Describe("test deployable label operations", func() {
	It("given a deployable and map, deployable should merge the entry from map", func() {
		dpl := dplv1.Deployable{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dpl-label",
			},
			Spec: dplv1.DeployableSpec{
				Template: &runtime.RawExtension{
					Object: &corev1.ConfigMap{},
				},
			},
		}

		var tests = []struct {
			given    map[string]string
			add      map[string]string
			expected map[string]string
		}{
			{map[string]string{}, map[string]string{"a": "b"}, map[string]string{"a": "b"}},
			{map[string]string{"a": "g"}, map[string]string{"a": "b"}, map[string]string{"a": "b"}},
			{map[string]string{"a": "g"}, map[string]string{"a": "b", "e": "f"}, map[string]string{"a": "b", "e": "f"}},
		}
		for _, tt := range tests {
			tt := tt
			tdpl := dpl.DeepCopy()
			tl := tdpl.GetLabels()
			tdpl.SetLabels(tl)
			utils.AddOrAppendChannelLabel(tdpl, tt.add)
			tlabel := tdpl.GetLabels()
			Expect(cmp.Equal(tlabel, tt.expected)).Should(BeTrue())
		}
	})
})
