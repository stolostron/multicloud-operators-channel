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

package helmrepo

import (
	"testing"
	"time"

	tlog "github.com/go-logr/logr/testing"
	"github.com/onsi/gomega"
	"golang.org/x/net/context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"
	synchronizer "github.com/open-cluster-management/multicloud-operators-channel/pkg/synchronizer/helmreposynchronizer"
)

var c client.Client

var targetNamespace = "default"

var targetChannelName = "foo"

var targetChannelType = chv1.ChannelType("Namespace")

var expectedRequest = reconcile.Request{
	NamespacedName: types.NamespacedName{
		Name: targetChannelName, Namespace: targetNamespace}}

const timeout = time.Second * 10

func TestHelmRepoReconcile(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Create new channelInstance
	channelInstance := &chv1.Channel{
		ObjectMeta: metav1.ObjectMeta{
			Name:      targetChannelName,
			Namespace: targetNamespace},
		Spec: chv1.ChannelSpec{
			Type:     targetChannelType,
			Pathname: targetNamespace,
		},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{})

	g.Expect(err).NotTo(gomega.HaveOccurred())

	c = mgr.GetClient()

	recFn, requests := SetupTestReconcile(newReconciler(mgr, &synchronizer.ChannelSynchronizer{}, tlog.NullLogger{}))
	g.Expect(add(mgr, recFn, tlog.NullLogger{})).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	// Create the Channel  object and expect the Reconcile
	g.Expect(c.Create(context.TODO(), channelInstance)).NotTo(gomega.HaveOccurred())

	defer c.Delete(context.TODO(), channelInstance)

	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
}
