// Copyright 2021 The Kubernetes Authors.
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
	"os"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func TestWireupWebhook(t *testing.T) {
	g := NewGomegaWithT(t)

	mgr, err := manager.New(cfg, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(HaveOccurred())

	k8sClient = mgr.GetClient()

	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Minute)
	mgrStopped := StartTestManager(ctx, mgr, g)

	defer func() {
		cancel()
		mgrStopped.Wait()
	}()

	g.Expect(mgr.GetCache().WaitForCacheSync(ctx)).Should(BeTrue())

	testNs := "default"
	os.Setenv("POD_NAMESPACE", testNs)
	os.Setenv("DEPLOYMENT_LABEL", testNs)

	wbhName := "test-wbh"
	wbhNameSetUp := func(w *WireUp) {
		w.WebhookName = wbhName
	}

	wireUp, err := NewWireUp(context.TODO(), mgr, wbhNameSetUp)
	Expect(err).NotTo(HaveOccurred())

	caCert, err := wireUp.Attach(k8sClient)
	Expect(err).NotTo(HaveOccurred())

	err = DelPreValiationCfg20(k8sClient)
	Expect(err).NotTo(HaveOccurred())

	err = wireUp.WireUpWebhookSupplymentryResource(false, k8sClient, caCert,
		schema.GroupVersionKind{Group: "", Version: "v1", Kind: "channels"},
		[]admissionv1.OperationType{admissionv1.Create})
	Expect(err).NotTo(HaveOccurred())

	err = wireUp.WireUpWebhookSupplymentryResource(true, k8sClient, caCert,
		schema.GroupVersionKind{Group: "", Version: "v1", Kind: "channels"},
		[]admissionv1.OperationType{admissionv1.Create})
	Expect(err).NotTo(HaveOccurred())

	wbhSvc := &corev1.Service{}
	svcKey := wireUp.WebHookeSvcKey
	Expect(mgr.GetClient().Get(context.TODO(), svcKey, wbhSvc)).Should(Succeed())

	defer func() {
		Expect(mgr.GetClient().Delete(context.TODO(), wbhSvc)).Should(Succeed())
	}()

	wbhCfg := &admissionv1.ValidatingWebhookConfiguration{}
	cfgKey := types.NamespacedName{Name: GetValidatorName(wbhName)}
	Expect(mgr.GetClient().Get(context.TODO(), cfgKey, wbhCfg)).Should(Succeed())

	defer func() {
		Expect(mgr.GetClient().Delete(context.TODO(), wbhCfg)).Should(Succeed())
	}()
}
