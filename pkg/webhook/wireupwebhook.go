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
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	gerr "github.com/pkg/errors"

	appsv1 "k8s.io/api/apps/v1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	admissionregistration "k8s.io/api/admissionregistration/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"
)

const (
	tlsCrt = "tls.crt"
	tlsKey = "tls.key"
	caCrt  = "ca.crt"

	validatorPath = "/validate-apps-open-cluster-management-io-v1-channel"
	webhookPort   = 9443

	podNamespaceEnvVar = "POD_NAMESPACE"
	operatorNameEnvVar = "OPERATOR_NAME"

	webhookName          = "vchannel.kb.io"
	webhookValidatorName = "channel-webhook-validator"
	webhookServiceName   = "channel-webhook-svc"

	webhookSecretName = "channel-webhook-server-cert"
	webhookAnno       = "service.alpha.openshift.io/serving-cert-secret-name"
	resourceName      = "channels"
)

var log = logf.Log.WithName("operator-channnel-webhook")

//assuming we have a service set up for the webhook, and the service is linking
//to a secret which has the CA
func WireUpWebhookWithKube(clt client.Client, whk *webhook.Server) error {
	certDir := filepath.Join("/tmp", "channel-webhookcert")
	// grab
	podNs, err := findEnvVariable(podNamespaceEnvVar)
	if err != nil {
		return gerr.Wrap(err, "failed to wire up webhook with kube")
	}

	//leverage ocp to generate cert secret
	if err := createWebhookService(clt, podNs); err != nil {
		return gerr.Wrap(err, "failed to wire up webhook with kube")
	}

	caCert, err := parseServiceSecret(clt, certDir, podNs)
	if err != nil {
		return gerr.Wrap(err, "failed to wire up webhook with kube")
	}

	if err := createOrUpdateValiatingWebhook(clt, podNs, validatorPath, caCert); err != nil {
		return gerr.Wrap(err, "failed to wire up webhook with kube")
	}

	whk.Port = webhookPort
	whk.CertDir = certDir

	log.Info("registering webhooks to the webhook server")
	whk.Register(validatorPath, &webhook.Admission{Handler: &ChannelValidator{Client: clt}})

	return nil
}

func findEnvVariable(envName string) (string, error) {
	val, found := os.LookupEnv(envName)
	if !found {
		return "", fmt.Errorf("%s env var is not set", envName)
	}

	return val, nil
}

func parseServiceSecret(clt client.Client, certDir, ns string) ([]byte, error) {
	srt := &corev1.Secret{}

	var ca []byte

	srtKey := types.NamespacedName{Name: webhookSecretName, Namespace: ns}

	if err := clt.Get(context.TODO(), srtKey, srt); err != nil {
		return ca, err
	}

	if err := os.MkdirAll(certDir, os.ModePerm); err != nil {
		return ca, err
	}

	if len(srt.Data) == 0 {
		return ca, gerr.New("service secret doen't contain tls info")
	}

	if err := ioutil.WriteFile(filepath.Join(certDir, tlsCrt), srt.Data[tlsCrt], os.FileMode(0644)); err != nil {
		return ca, err
	}

	if err := ioutil.WriteFile(filepath.Join(certDir, tlsKey), srt.Data[tlsKey], os.FileMode(0644)); err != nil {
		return ca, err
	}

	ca = srt.Data[caCrt]

	return ca, nil
}

func createWebhookService(c client.Client, namespace string) error {
	service := &corev1.Service{}
	key := types.NamespacedName{Name: webhookServiceName, Namespace: namespace}

	for {
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

			switch err.(type) {
			case *cache.ErrCacheNotStarted:
				time.Sleep(time.Second)
				continue
			default:
				return gerr.Wrap(err, fmt.Sprintf("Failed to get %s/%s service", namespace, webhookServiceName))
			}
		}

		log.Info(fmt.Sprintf("%s/%s service is found", namespace, webhookServiceName))

		return nil
	}
}

func createOrUpdateValiatingWebhook(c client.Client, namespace, path string, ca []byte) error {
	validator := &admissionregistration.ValidatingWebhookConfiguration{}
	key := types.NamespacedName{Name: webhookValidatorName}

	for {
		if err := c.Get(context.TODO(), key, validator); err != nil {
			if errors.IsNotFound(err) {
				cfg := newValidatingWebhookCfg(namespace, path, ca)

				setOwnerReferences(c, namespace, cfg)

				if err := c.Create(context.TODO(), cfg); err != nil {
					return gerr.Wrap(err, fmt.Sprintf("Failed to create validating webhook %s", webhookValidatorName))
				}

				log.Info(fmt.Sprintf("Create validating webhook %s", webhookValidatorName))

				return nil
			}

			switch err.(type) {
			case *cache.ErrCacheNotStarted:
				time.Sleep(time.Second)
				continue
			default:
				return gerr.Wrap(err, fmt.Sprintf("Failed to get validating webhook %s", webhookValidatorName))
			}
		}

		validator.Webhooks[0].ClientConfig.Service.Namespace = namespace
		validator.Webhooks[0].ClientConfig.CABundle = ca

		if err := c.Update(context.TODO(), validator); err != nil {
			return gerr.Wrap(err, fmt.Sprintf("Failed to update validating webhook %s", webhookValidatorName))
		}

		log.Info(fmt.Sprintf("Update validating webhook %s", webhookValidatorName))

		return nil
	}
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
			Annotations: map[string]string{
				webhookAnno: webhookServiceName,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports:    []corev1.ServicePort{{Port: 443, TargetPort: intstr.FromInt(9443)}},
			Selector: map[string]string{"name": operatorName},
		},
	}, nil
}

func newValidatingWebhookCfg(namespace, path string, ca []byte) *admissionregistration.ValidatingWebhookConfiguration {
	return &admissionregistration.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: webhookValidatorName,
		},

		Webhooks: []admissionregistration.ValidatingWebhook{{
			Name: webhookName,
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
			ClientConfig: admissionregistration.WebhookClientConfig{
				Service: &admissionregistration.ServiceReference{
					Name:      webhookServiceName,
					Namespace: namespace,
					Path:      &path,
				},
				CABundle: ca,
			},
		}},
	}
}
