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
	"testing"

	k8scertutil "k8s.io/client-go/util/cert"
)

func TestGenerateSignedWebhookCertificates(t *testing.T) {
	podNamespaceEnvVar := "POD_NAMESPACE"
	webhookServiceName := "default"

	os.Setenv(podNamespaceEnvVar, "test")

	certDir := "/tmp/tmp-cert"

	defer func() {
		os.RemoveAll(certDir)
		os.Unsetenv(podNamespaceEnvVar)
	}()

	podNs, err := findEnvVariable(podNamespaceEnvVar)
	if err != nil {
		t.Errorf("failed to get the pod namespace, %v", err)
	}

	ca, err := GenerateWebhookCerts(certDir, podNs, webhookServiceName)
	if err != nil {
		t.Errorf("Generate signed certificate failed, %v", err)
	}

	if ca == nil {
		t.Errorf("Generate signed certificate failed")
	}

	canReadCertAndKey, err := k8scertutil.CanReadCertAndKey("/tmp/tmp-cert/tls.crt", "/tmp/tmp-cert/tls.key")
	if err != nil {
		t.Errorf("Generate signed certificate failed, %v", err)
	}

	if !canReadCertAndKey {
		t.Errorf("Generate signed certificate failed")
	}
}
