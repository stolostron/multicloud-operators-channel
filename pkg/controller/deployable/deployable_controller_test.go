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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"
	dplv1 "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis/apps/v1"
)

const (
	targetNamespace      = "default"
	targetChannelName    = "channl-deployable-reconcile"
	targetChannelType    = chv1.ChannelType("namespace")
	targetDeployableName = "t-deployable"
	k8swait              = time.Second * 5
)

var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: targetDeployableName, Namespace: targetNamespace}}

var _ = Describe("origin test case", func() {
	It("should pass", func() {
		channelInstance := &chv1.Channel{
			ObjectMeta: metav1.ObjectMeta{
				Name:      targetChannelName,
				Namespace: targetNamespace},
			Spec: chv1.ChannelSpec{
				Type:     targetChannelType,
				Pathname: targetNamespace,
			},
		}

		deployableInstance := &dplv1.Deployable{
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

		//create events handler on hub cluster. All the deployable events will be written to the root deploable on hub cluster.
		hubClientSet, err := kubernetes.NewForConfig(k8sManager.GetConfig())
		Expect(err).NotTo(HaveOccurred())

		eventBroadcaster := record.NewBroadcaster()
		eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: hubClientSet.CoreV1().Events("")})
		recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "channel"})

		recFn, requests := SetupTestReconcile(newReconciler(k8sManager, recorder, tlog.NullLogger{}))
		Expect(add(k8sManager, recFn, tlog.NullLogger{})).NotTo(HaveOccurred())

		// Create the Channel object and expect the Reconcile
		Expect(k8sClient.Create(context.TODO(), channelInstance)).NotTo(HaveOccurred())

		defer func() {
			Expect(k8sClient.Delete(context.TODO(), channelInstance)).Should(Succeed())
		}()

		time.Sleep(k8swait)
		// if there's no deployables, then we won't have any reconcile requests
		Eventually(requests, k8swait).ShouldNot(Receive(Equal(expectedRequest)))

		Expect(k8sClient.Create(context.TODO(), deployableInstance)).NotTo(HaveOccurred())

		defer func() {
			Expect(k8sClient.Delete(context.TODO(), deployableInstance)).Should(Succeed())
		}()
		time.Sleep(k8swait)

		//if there's deployables, then the channel update will generate requests on all the existing deployables
		Eventually(requests, k8swait).Should(Receive(Equal(expectedRequest)))
	})
})
