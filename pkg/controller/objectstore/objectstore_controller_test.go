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
	"testing"
	"time"

	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/IBM/multicloud-operators-channel/pkg/utils"
	dplv1alpha1 "github.com/IBM/multicloud-operators-deployable/pkg/apis/app/v1alpha1"
)

const timeout = time.Second * 2

func TestObjstoreController(t *testing.T) {
	testNamespace := "a-ns"
	testDplName := "tdpl"
	theAnno := map[string]string{"who": "bear"}

	expectedResult := reconcile.Result{}

	g := gomega.NewGomegaWithT(t)

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(gomega.HaveOccurred())

	chDesc, err := utils.CreateChannelDescriptor()
	g.Expect(err).NotTo(gomega.HaveOccurred())

	rec := newReconciler(mgr, chDesc)
	recFn, result := SetupTestReconcile(rec)

	g.Expect(add(mgr, recFn)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	c := mgr.GetClient()
	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	dpl := &dplv1alpha1.Deployable{
		ObjectMeta: metav1.ObjectMeta{
			Name:        testDplName,
			Namespace:   testNamespace,
			Annotations: theAnno,
		},
		Spec: dplv1alpha1.DeployableSpec{
			Template: &runtime.RawExtension{
				Object: &v1.ConfigMap{},
			},
		},
	}

	ctx := context.TODO()
	g.Expect(c.Create(ctx, dpl)).NotTo(gomega.HaveOccurred())
	defer c.Delete(ctx, dpl)

	g.Eventually(result, timeout).Should(gomega.Receive(gomega.Equal(expectedResult)))
}
