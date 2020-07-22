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
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	mgr "sigs.k8s.io/controller-runtime/pkg/manager"
)

var _ = Describe("test if webhook's supplymentryResource create properly", func() {
	// somehow this test only fail on travis
	// make sure this one runs at the end, otherwise, we might register this
	// webhook before the default one, which cause unexpected results.
	PContext("given a k8s env, it create svc and validating webhook config", func() {
		var (
			lMgr    mgr.Manager
			certDir string
			testNs  string
			caCert  []byte
			err     error
			sstop   chan struct{}
		)

		It("should create a service and ValidatingWebhookConfiguration", func() {
			lMgr, err = mgr.New(testEnv.Config, mgr.Options{MetricsBindAddress: "0"})
			Expect(err).Should(BeNil())

			sstop = make(chan struct{})
			defer close(sstop)
			go func() {
				Expect(lMgr.Start(sstop)).Should(Succeed())
			}()

			certDir = filepath.Join(os.TempDir(), "k8s-webhook-server", "serving-certs")
			testNs = "default"
			os.Setenv("POD_NAMESPACE", testNs)

			podNs, err := findEnvVariable(podNamespaceEnvVar)
			Expect(err).Should(BeNil())

			caCert, err = GenerateWebhookCerts(certDir, podNs, WebhookServiceName)
			Expect(err).Should(BeNil())
			validatorName := "test-validator"
			wbhSvcNm := "ch-wbh-svc"
			WireUpWebhookSupplymentryResource(lMgr, stop, wbhSvcNm, validatorName, certDir, caCert,
				schema.GroupVersionKind{Group: "", Version: "v1", Kind: "channels"},
				[]admissionv1.OperationType{admissionv1.Create})

			time.Sleep(3 * time.Second)
			wbhSvc := &corev1.Service{}
			svcKey := types.NamespacedName{Name: wbhSvcNm, Namespace: podNs}
			Expect(lMgr.GetClient().Get(context.TODO(), svcKey, wbhSvc)).Should(Succeed())
			defer func() {
				Expect(lMgr.GetClient().Delete(context.TODO(), wbhSvc)).Should(Succeed())
			}()

			wbhCfg := &admissionv1.ValidatingWebhookConfiguration{}
			cfgKey := types.NamespacedName{Name: validatorName}
			Expect(lMgr.GetClient().Get(context.TODO(), cfgKey, wbhCfg)).Should(Succeed())

			defer func() {
				Expect(lMgr.GetClient().Delete(context.TODO(), wbhCfg)).Should(Succeed())
			}()
		})
	})
})
