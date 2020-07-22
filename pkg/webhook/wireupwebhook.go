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
	"k8s.io/apimachinery/pkg/runtime/schema"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

const (
	tlsCrt = "tls.crt"
	tlsKey = "tls.key"

	WebhookPort          = 9443
	ValidatorPath        = "/v1-validate"
	WebhookValidatorName = "channel-webhook-validator"
	WebhookServiceName   = "multicluster-operators-channel-svc"

	podNamespaceEnvVar = "POD_NAMESPACE"
	// acm is using `app: multicluster-operators-application` as pod label
	deployLabelEnvVar = "DEPLOYMENT_LABEL"

	deploySelectorName = "app"

	webhookName = "channels.apps.open-cluster-management.webhook"

	resourceName = "channels"
)

var log = logf.Log.WithName("operator-channnel-webhook")

func WireUpWebhook(clt client.Client, whk *webhook.Server, certDir string) ([]byte, error) {
	whk.Port = WebhookPort
	whk.CertDir = certDir

	log.Info("registering webhooks to the webhook server")
	whk.Register(ValidatorPath, &webhook.Admission{Handler: &ChannelValidator{Client: clt}})

	podNs, err := findEnvVariable(podNamespaceEnvVar)
	if err != nil {
		return []byte{}, gerr.Wrap(err, "faild to get pod namespace ")
	}

	return GenerateWebhookCerts(certDir, podNs, WebhookServiceName)
}

//assuming we have a service set up for the webhook, and the service is linking
//to a secret which has the CA
func WireUpWebhookSupplymentryResource(mgr manager.Manager, stop <-chan struct{}, wbhSvcName, validatorName, certDir string,
	caCert []byte, gvk schema.GroupVersionKind, ops []admissionv1.OperationType) {
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

	if err := createWebhookService(clt, wbhSvcName, podNs); err != nil {
		log.Error(err, "failed to wire up webhook with kube")
		os.Exit(1)
	}

	if err := createOrUpdateValiatingWebhook(clt, wbhSvcName, validatorName, podNs, ValidatorPath, caCert, gvk, ops); err != nil {
		log.Error(err, "failed to wire up webhook with kube")
		os.Exit(1)
	}
}

func findEnvVariable(envName string) (string, error) {
	val, found := os.LookupEnv(envName)
	if !found {
		return "", fmt.Errorf("%s env var is not set", envName)
	}

	return val, nil
}

func createWebhookService(c client.Client, wbhSvcName, namespace string) error {
	service := &corev1.Service{}
	key := types.NamespacedName{Name: wbhSvcName, Namespace: namespace}

	if err := c.Get(context.TODO(), key, service); err != nil {
		if errors.IsNotFound(err) {
			service, err := newWebhookService(wbhSvcName, namespace)
			if err != nil {
				return gerr.Wrap(err, "failed to create service for webhook")
			}

			setOwnerReferences(c, namespace, service)

			if err := c.Create(context.TODO(), service); err != nil {
				return err
			}

			log.Info(fmt.Sprintf("Create %s/%s service", namespace, wbhSvcName))

			return nil
		}
	}

	log.Info(fmt.Sprintf("%s/%s service is found", namespace, wbhSvcName))

	return nil
}

func createOrUpdateValiatingWebhook(c client.Client, wbhSvcName, validatorName, namespace, path string, ca []byte,
	gvk schema.GroupVersionKind, ops []admissionv1.OperationType) error {
	validator := &admissionv1.ValidatingWebhookConfiguration{}
	key := types.NamespacedName{Name: validatorName}

	if err := c.Get(context.TODO(), key, validator); err != nil {
		if errors.IsNotFound(err) {
			cfg := newValidatingWebhookCfg(wbhSvcName, validatorName, namespace, path, ca, gvk, ops)

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
	deployLabel, err := findEnvVariable(deployLabelEnvVar)
	if err != nil {
		return
	}

	key := types.NamespacedName{Name: deployLabel, Namespace: namespace}
	owner := &appsv1.Deployment{}

	if err := c.Get(context.TODO(), key, owner); err != nil {
		log.Error(err, fmt.Sprintf("Failed to set owner references for %s", obj.GetName()))
		return
	}

	obj.SetOwnerReferences([]metav1.OwnerReference{
		*metav1.NewControllerRef(owner, owner.GetObjectKind().GroupVersionKind())})
}

func newWebhookService(wbhSvcName, namespace string) (*corev1.Service, error) {
	deployLabel, err := findEnvVariable(deployLabelEnvVar)
	if err != nil {
		return nil, err
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      wbhSvcName,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port:       443,
					TargetPort: intstr.FromInt(WebhookPort),
				},
			},
			Selector: map[string]string{deploySelectorName: deployLabel},
		},
	}, nil
}

func newValidatingWebhookCfg(wbhSvcName, validatorName, namespace, path string, ca []byte,
	gvk schema.GroupVersionKind, ops []admissionv1.OperationType) *admissionv1.ValidatingWebhookConfiguration {
	fail := admissionv1.Fail
	side := admissionv1.SideEffectClassNone

	return &admissionv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: validatorName,
		},

		Webhooks: []admissionv1.ValidatingWebhook{{
			Name:                    webhookName,
			AdmissionReviewVersions: []string{"v1beta1"},
			SideEffects:             &side,
			FailurePolicy:           &fail,
			ClientConfig: admissionv1.WebhookClientConfig{
				Service: &admissionv1.ServiceReference{
					Name:      wbhSvcName,
					Namespace: namespace,
					Path:      &path,
				},
				CABundle: ca,
			},
			Rules: []admissionv1.RuleWithOperations{{
				Rule: admissionv1.Rule{
					APIGroups:   []string{gvk.Group},
					APIVersions: []string{gvk.Version},
					Resources:   []string{gvk.Kind},
				},
				Operations: ops,
			},
			}},
		},
	}
}
