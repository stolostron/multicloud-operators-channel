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
	"fmt"
	"os"

	gerr "github.com/pkg/errors"

	appsv1 "k8s.io/api/apps/v1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	admissionregistration "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"
)

const (
	tlsCrt = "tls.crt"
	tlsKey = "tls.key"

	WebhookPort          = 9443
	ValidatorPath        = "/validate-apps-open-cluster-management-io-v1-channel"
	WebhookValidatorName = "channel-webhook-validator"

	podNamespaceEnvVar = "POD_NAMESPACE"
	operatorNameEnvVar = "OPERATOR_NAME"

	webhookName        = "vchannel.kb.io"
	webhookServiceName = "channel-webhook-svc"

	resourceName = "channels"
)

var log = logf.Log.WithName("operator-channnel-webhook")

func WireUpWebhook(clt client.Client, whk *webhook.Server, certDir string) ([]byte, error) {
	whk.Port = WebhookPort
	whk.CertDir = certDir

	log.Info("registering webhooks to the webhook server")
	whk.Register(ValidatorPath, &webhook.Admission{Handler: &ChannelValidator{Client: clt}})

	return GenerateWebhookCerts(certDir)
}

//assuming we have a service set up for the webhook, and the service is linking
//to a secret which has the CA
func WireUpWebhookSupplymentryResource(mgr manager.Manager, stop <-chan struct{}, validatorName, certDir string, caCert []byte) {
	log.Info("entry wire up webhook")
	defer log.Info("exit wire up webhook ")

	podNs, err := findEnvVariable(podNamespaceEnvVar)
	if err != nil {
		log.Error(err, "failed to wire up webhook with kube")
	}

	if !mgr.GetCache().WaitForCacheSync(stop) {
		log.Error(gerr.New("cache not started"), "failed to start up cache")
	}

	log.Info("cache is ready to consume")

	clt := mgr.GetClient()

	if err := createWebhookService(clt, podNs); err != nil {
		log.Error(err, "failed to wire up webhook with kube")
	}

	if err := createOrUpdateValiatingWebhook(clt, validatorName, podNs, ValidatorPath, caCert); err != nil {
		log.Error(err, "failed to wire up webhook with kube")
	}
}

func findEnvVariable(envName string) (string, error) {
	val, found := os.LookupEnv(envName)
	if !found {
		return "", fmt.Errorf("%s env var is not set", envName)
	}

	return val, nil
}

func createWebhookService(c client.Client, namespace string) error {
	service := &corev1.Service{}
	key := types.NamespacedName{Name: webhookServiceName, Namespace: namespace}

	if err := c.Get(context.TODO(), key, service); err != nil {
		if errors.IsNotFound(err) {
			service, err := newWebhookService(namespace)
			if err != nil {
				return gerr.Wrap(err, "failed to create service for webhook")
			}

			setOwnerReferences(c, namespace, service)

			if err := c.Create(context.TODO(), service); err != nil {
				return err
			}

			log.Info(fmt.Sprintf("Create %s/%s service", namespace, webhookServiceName))

			return nil
		}
	}

	log.Info(fmt.Sprintf("%s/%s service is found", namespace, webhookServiceName))

	return nil
}

func createOrUpdateValiatingWebhook(c client.Client, validatorName, namespace, path string, ca []byte) error {
	validator := &admissionregistration.ValidatingWebhookConfiguration{}
	key := types.NamespacedName{Name: validatorName}

	if err := c.Get(context.TODO(), key, validator); err != nil {
		if errors.IsNotFound(err) {
			cfg := newValidatingWebhookCfg(validatorName, namespace, path, ca)

			setOwnerReferences(c, namespace, cfg)

			if err := c.Create(context.TODO(), cfg); err != nil {
				return gerr.Wrap(err, fmt.Sprintf("Failed to create validating webhook %s", validatorName))
			}

			log.Info(fmt.Sprintf("Create validating webhook %s", validatorName))

			return nil
		}
	}

	validator.Webhooks[0].ClientConfig.Service.Namespace = namespace
	validator.Webhooks[0].ClientConfig.CABundle = ca

	if err := c.Update(context.TODO(), validator); err != nil {
		return gerr.Wrap(err, fmt.Sprintf("Failed to update validating webhook %s", validatorName))
	}

	log.Info(fmt.Sprintf("Update validating webhook %s", validatorName))

	return nil
}

func setOwnerReferences(c client.Client, namespace string, obj metav1.Object) {
	operatorName, err := findEnvVariable(operatorNameEnvVar)
	if err != nil {
		return
	}

	key := types.NamespacedName{Name: operatorName, Namespace: namespace}
	owner := &appsv1.Deployment{}

	if err := c.Get(context.TODO(), key, owner); err != nil {
		log.Error(err, fmt.Sprintf("Failed to set owner references for %s", obj.GetName()))
		return
	}

	obj.SetOwnerReferences([]metav1.OwnerReference{
		*metav1.NewControllerRef(owner, owner.GetObjectKind().GroupVersionKind())})
}

func newWebhookService(namespace string) (*corev1.Service, error) {
	operatorName, err := findEnvVariable(operatorNameEnvVar)
	if err != nil {
		return nil, err
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      webhookServiceName,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port:       443,
					TargetPort: intstr.FromInt(WebhookPort),
				},
			},
			Selector: map[string]string{"name": operatorName},
		},
	}, nil
}

func newValidatingWebhookCfg(validatorName, namespace, path string, ca []byte) *admissionregistration.ValidatingWebhookConfiguration {
	fail := admissionregistration.Fail
	side := admissionregistration.SideEffectClassNone

	return &admissionregistration.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: validatorName,
		},

		Webhooks: []admissionregistration.ValidatingWebhook{{
			Name:                    webhookName,
			AdmissionReviewVersions: []string{"v1beta1"},
			SideEffects:             &side,
			FailurePolicy:           &fail,
			ClientConfig: admissionregistration.WebhookClientConfig{
				Service: &admissionregistration.ServiceReference{
					Name:      webhookServiceName,
					Namespace: namespace,
					Path:      &path,
				},
				CABundle: ca,
			},
			Rules: []admissionregistration.RuleWithOperations{{
				Rule: admissionregistration.Rule{
					APIGroups:   []string{chv1.SchemeGroupVersion.Group},
					APIVersions: []string{chv1.SchemeGroupVersion.Version},
					Resources:   []string{resourceName},
				},
				Operations: []admissionregistration.OperationType{
					admissionregistration.Create,
					admissionregistration.Update,
				},
			}},
		}},
	}
}
