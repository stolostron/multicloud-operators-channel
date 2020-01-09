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
	"testing"
	"time"

	"github.com/onsi/gomega"
	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appv1alpha1 "github.com/IBM/multicloud-operators-channel/pkg/apis/app/v1alpha1"
	dplv1alpha1 "github.com/IBM/multicloud-operators-deployable/pkg/apis/app/v1alpha1"
)

var c client.Client

var targetNamespace = "default"

var targetChannelName = "channl-deployable-reconcile"

var targetChannelType = appv1alpha1.ChannelType("namespace")

var targetDeployableName = "t-deployable"
var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: targetDeployableName, Namespace: targetNamespace}}

const timeout = time.Second * 5

func TestDeployableReconcile(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	channelInstance := &appv1alpha1.Channel{
		ObjectMeta: metav1.ObjectMeta{
			Name:      targetChannelName,
			Namespace: targetNamespace},
		Spec: appv1alpha1.ChannelSpec{
			Type:     targetChannelType,
			PathName: targetNamespace,
		},
	}

	deployableInstance := &dplv1alpha1.Deployable{
		ObjectMeta: metav1.ObjectMeta{
			Name:      targetDeployableName,
			Namespace: targetNamespace,
		},
		Spec: dplv1alpha1.DeployableSpec{
			Template: &runtime.RawExtension{
				Object: &v1.ConfigMap{},
			},
		},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(gomega.HaveOccurred())

	c = mgr.GetClient()

	//create events handler on hub cluster. All the deployable events will be written to the root deploable on hub cluster.
	hubClientSet, err := kubernetes.NewForConfig(cfg)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: hubClientSet.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "channel"})

	recFn, requests := SetupTestReconcile(newReconciler(mgr, recorder))
	g.Expect(add(mgr, recFn)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	// Create the Channel object and expect the Reconcile
	g.Expect(c.Create(context.TODO(), channelInstance)).NotTo(gomega.HaveOccurred())

	defer c.Delete(context.TODO(), channelInstance)

	time.Sleep(time.Second * 1)
	// if there's no deployables, then we won't have any reconcile requests
	g.Eventually(requests, timeout).ShouldNot(gomega.Receive(gomega.Equal(expectedRequest)))

	g.Expect(c.Create(context.TODO(), deployableInstance)).NotTo(gomega.HaveOccurred())

	defer c.Delete(context.TODO(), deployableInstance)
	time.Sleep(time.Second * 1)

	//if there's deployables, then the channel update will generate requests on all the existing deployables
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
}
