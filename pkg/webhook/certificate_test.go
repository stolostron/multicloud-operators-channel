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
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	k8scertutil "k8s.io/client-go/util/cert"
)

var _ = FDescribe("self-signed cert", func() {
	var (
		podNamespaceEnvVar = "POD_NAMESPACE"
		webhookServiceName = "default"
		certDir            = "/tmp/tmp-cert"
	)

	It("should create CA and store it in secret, private key pairs should be created as well", func() {
		os.Setenv(podNamespaceEnvVar, "test")
		defer func() {
			os.RemoveAll(certDir)
			os.Unsetenv(podNamespaceEnvVar)
		}()

		podNs, err := findEnvVariable(podNamespaceEnvVar)
		Expect(err).Should(Succeed())

		ca, err := GenerateWebhookCerts(k8sClient, certDir, podNs, webhookServiceName)
		Expect(err).Should(Succeed())
		Expect(ca).ShouldNot(BeNil())

		isReadCertAndKey, err := k8scertutil.CanReadCertAndKey("/tmp/tmp-cert/tls.crt", "/tmp/tmp-cert/tls.key")
		Expect(err).Should(Succeed())
		Expect(isReadCertAndKey).Should(BeTrue())

	})

})
