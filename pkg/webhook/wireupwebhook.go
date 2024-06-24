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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gerr "github.com/pkg/errors"

	appsv1 "k8s.io/api/apps/v1"
	apixv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/go-logr/logr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

type WireUp struct {
	mgr manager.Manager
	ctx context.Context

	Server  webhook.Server
	Handler webhook.AdmissionHandler
	CertDir string
	logr.Logger

	WebhookName string
	WebHookPort int

	WebHookeSvcKey     types.NamespacedName
	WebHookServicePort int

	ValidtorPath string

	DeployLabel        string
	DeploymentSelector map[string]string
}

func NewWireUp(ctx context.Context, mgr manager.Manager,
	opts ...func(*WireUp)) (*WireUp, error) {
	WebhookName := "channels-apps-open-cluster-management-webhook"

	deployLabelEnvVar := "DEPLOYMENT_LABEL"
	deploymentLabel, err := findEnvVariable(deployLabelEnvVar)

	if err != nil {
		return nil, gerr.Wrap(err, "failed to create a new webhook wireup")
	}

	deploymentSelectKey := "app"

	deploymentSelector := map[string]string{deploymentSelectKey: deploymentLabel}

	podNamespaceEnvVar := "POD_NAMESPACE"

	podNamespace, err := findEnvVariable(podNamespaceEnvVar)
	if err != nil {
		return nil, gerr.Wrap(err, "failed to create a new webhook wireup")
	}

	wireUp := &WireUp{
		mgr:    mgr,
		ctx:    ctx,
		Server: mgr.GetWebhookServer(),
		Logger: logf.Log.WithName("webhook"),

		WebhookName:        WebhookName,
		WebHookPort:        9443,
		WebHookServicePort: 443,
		CertDir:            filepath.Join(os.TempDir(), "k8s-webhook-server", "serving-certs"),

		ValidtorPath:       "/duplicate-validation",
		WebHookeSvcKey:     types.NamespacedName{Name: GetWebHookServiceName(WebhookName), Namespace: podNamespace},
		DeployLabel:        deploymentLabel,
		DeploymentSelector: deploymentSelector,
	}

	for _, op := range opts {
		op(wireUp)
	}

	return wireUp, nil
}

func GetValidatorName(wbhName string) string {
	//domain style, separate by dots
	return strings.ReplaceAll(wbhName, "-", ".") + ".validator"
}

func GetWebHookServiceName(wbhName string) string {
	//k8s resrouce nameing, separate by '-'
	return wbhName + "-svc"
}

func (w *WireUp) Attach(clt client.Client) ([]byte, error) {
	w.Logger.Info("entry Attach webhook")
	defer w.Logger.Info("exit Attach webhook")

	w.Logger.Info("registering webhooks to the webhook server")
	w.Server.Register(w.ValidtorPath, &webhook.Admission{Handler: w.Handler})

	return GenerateWebhookCerts(clt, w.CertDir, w.WebHookeSvcKey.Namespace, w.WebHookeSvcKey.Name)
}

type CleanUpFunc func(client.Client) error

func DelPreValiationCfg20(clt client.Client) error {
	pCfg := &admissionv1.ValidatingWebhookConfiguration{}
	pCfgKey := types.NamespacedName{Name: "channel-webhook-validator"}
	ctx := context.TODO()

	if err := clt.Get(ctx, pCfgKey, pCfg); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}

		return err
	}

	return clt.Delete(ctx, pCfg)
}

// assuming we have a service set up for the webhook, and the service is linking
// to a secret which has the CA
func (w *WireUp) WireUpWebhookSupplymentryResource(isExternalAPIServer bool, inClient client.Client,
	caCert []byte, gvk schema.GroupVersionKind, ops []admissionv1.OperationType, cFuncs ...CleanUpFunc) error {
	w.Logger.Info("entry wire up webhook resources")
	defer w.Logger.Info("exit wire up webhook resources")

	if !w.mgr.GetCache().WaitForCacheSync(w.ctx) {
		w.Logger.Error(gerr.New("cache not started"), "failed to start up cache")
	}

	w.Logger.Info("cache is ready to consume")

	for _, cf := range cFuncs {
		if err := cf(w.mgr.GetClient()); err != nil {
			return gerr.Wrap(err, "failed to clean up")
		}
	}

	return gerr.Wrap(w.createOrUpdateValiationWebhook(isExternalAPIServer, inClient, caCert, gvk, ops), "failed to set up the validation webhook config")
}

func findEnvVariable(envName string) (string, error) {
	val, found := os.LookupEnv(envName)
	if !found {
		return "", fmt.Errorf("%s env var is not set", envName)
	}

	return val, nil
}

func (w *WireUp) getOrCreateWebhookService(isExternalAPIServer bool, inClusterClient client.Client) error {
	service := &corev1.Service{}

	outCLusterClient := w.mgr.GetClient()

	// 1. On the management cluster, create the service for exposing the channel webhook server (channel pod)
	err := inClusterClient.Get(context.TODO(), w.WebHookeSvcKey, service)

	if err == nil {
		// This channel container could be running in a different pod. Delete and re-create the service
		// to ensure that the service always points to the correct target pod.
		deleteErr := inClusterClient.Delete(context.TODO(), service)

		if deleteErr != nil {
			w.Logger.Error(gerr.New(fmt.Sprintf("failed to delete existing service %s", w.WebHookeSvcKey.String())),
				fmt.Sprintf("failed to delete existing service %s", w.WebHookeSvcKey.String()))

			return deleteErr
		}

		w.Logger.Info(fmt.Sprintf("deleted existing service %s", w.WebHookeSvcKey.String()))
	}

	w.Logger.Info(fmt.Sprintf("creating in Cluster service %s ", w.WebHookeSvcKey.String()))

	newService := newWebhookServiceTemplate(false, w.WebHookeSvcKey, w.WebHookPort, w.WebHookServicePort, w.DeploymentSelector)

	setOwnerReferences(inClusterClient, w.Logger, w.WebHookeSvcKey.Namespace, w.DeployLabel, newService)

	if err := inClusterClient.Create(context.TODO(), newService); err != nil {
		return err
	}

	w.Logger.Info(fmt.Sprintf("created in Cluster service %s ", w.WebHookeSvcKey.String()))

	// 2. If isExternalAPIServer = true, create the additional service in the hosted cluster
	if !isExternalAPIServer {
		return nil
	}

	service = &corev1.Service{}
	err = outCLusterClient.Get(context.TODO(), w.WebHookeSvcKey, service)

	if err == nil {
		// Delete and re-create the service to ensure that the service always points to the correct target pod.
		deleteErr := outCLusterClient.Delete(context.TODO(), service)

		if deleteErr != nil {
			w.Logger.Error(gerr.New(fmt.Sprintf("failed to delete existing service %s on hosted cluster", w.WebHookeSvcKey.String())),
				fmt.Sprintf("failed to delete existing service %s on hosted cluster", w.WebHookeSvcKey.String()))

			return deleteErr
		}

		w.Logger.Info(fmt.Sprintf("deleted existing service %s on hosted cluster", w.WebHookeSvcKey.String()))
	}

	newService = newWebhookServiceTemplate(true, w.WebHookeSvcKey, w.WebHookPort, w.WebHookServicePort, w.DeploymentSelector)

	if err := outCLusterClient.Create(context.TODO(), newService); err != nil {
		return err
	}

	w.Logger.Info(fmt.Sprintf("created hosted cluster service %s ", w.WebHookeSvcKey.String()))

	// 3. get the service Cluster IP of the webhook server running on the management cluster. The service is created in step 1

	service = &corev1.Service{}

	if err = inClusterClient.Get(context.TODO(), w.WebHookeSvcKey, service); err != nil {
		return err
	}

	serviceClusterIP := service.Spec.ClusterIP

	if serviceClusterIP == "" {
		return errors.New("no service Cluster IP found: " + w.WebHookeSvcKey.String())
	}

	// 4. If isExternalAPIServer = true, create the additional endpoint in the hosted cluster
	endpoint := &corev1.Endpoints{}
	err = outCLusterClient.Get(context.TODO(), w.WebHookeSvcKey, endpoint)

	if err == nil {
		// Delete and re-create the endpoint to ensure that the service always points to the correct endpoint
		deleteErr := outCLusterClient.Delete(context.TODO(), endpoint)

		if deleteErr != nil {
			w.Logger.Error(gerr.New(fmt.Sprintf("failed to delete existing endpoint %s on hosted cluster", w.WebHookeSvcKey.String())),
				fmt.Sprintf("failed to delete existing endpoint %s on hosted cluster", w.WebHookeSvcKey.String()))

			return deleteErr
		}

		w.Logger.Info(fmt.Sprintf("deleted existing endpoint %s on hosted cluster", w.WebHookeSvcKey.String()))
	}

	newEndpoint := newWebhookEndpointTemplate(w.WebHookeSvcKey, w.WebHookServicePort, serviceClusterIP)

	if err := outCLusterClient.Create(context.TODO(), newEndpoint); err != nil {
		return err
	}

	w.Logger.Info(fmt.Sprintf("created hosted cluster endpoint %s ", w.WebHookeSvcKey.String()))

	return nil
}

func (w *WireUp) createOrUpdateValiationWebhook(isExternalAPIServer bool, inClient client.Client,
	ca []byte, gvk schema.GroupVersionKind,
	ops []admissionv1.OperationType) error {
	validator := &admissionv1.ValidatingWebhookConfiguration{}
	key := types.NamespacedName{Name: GetValidatorName(w.WebhookName)}

	validatorName := GetValidatorName(w.WebhookName)

	if err := w.mgr.GetClient().Get(context.TODO(), key, validator); err != nil {
		if apierrors.IsNotFound(err) { // create a new validator
			cfg := newValidatingWebhookCfg(w.WebHookeSvcKey, validatorName, w.ValidtorPath, ca, gvk, ops)

			setWebhookOwnerReferences(w.mgr.GetClient(), w.Logger, cfg)

			if err := w.mgr.GetClient().Create(context.TODO(), cfg); err != nil {
				return gerr.Wrap(err, fmt.Sprintf("Failed to create validating webhook %s", validatorName))
			}

			w.Logger.Info(fmt.Sprintf("Create validating webhook %s", validatorName))
		}
	} else { // update the existing validator
		validator.Webhooks[0].ClientConfig.Service.Namespace = w.WebHookeSvcKey.Namespace
		validator.Webhooks[0].ClientConfig.Service.Name = w.WebHookeSvcKey.Name
		validator.Webhooks[0].ClientConfig.CABundle = ca

		ignore := admissionv1.Ignore
		timeoutSeconds := int32(30)

		validator.Webhooks[0].FailurePolicy = &ignore
		validator.Webhooks[0].TimeoutSeconds = &timeoutSeconds

		setWebhookOwnerReferences(w.mgr.GetClient(), w.Logger, validator)

		if err := w.mgr.GetClient().Update(context.TODO(), validator); err != nil {
			return gerr.Wrap(err, fmt.Sprintf("Failed to update validating webhook %s", validatorName))
		}

		w.Logger.Info(fmt.Sprintf("Update validating webhook %s", validatorName))
	}

	// make sure the service of the validator exists
	return gerr.Wrap(w.getOrCreateWebhookService(isExternalAPIServer, inClient), "failed to set up service for webhook")
}

func setOwnerReferences(c client.Client, logger logr.Logger, deployNs string, deployLabel string, obj metav1.Object) {
	key := types.NamespacedName{Name: deployLabel, Namespace: deployNs}
	owner := &appsv1.Deployment{}

	if err := c.Get(context.TODO(), key, owner); err != nil {
		logger.Error(err, fmt.Sprintf("Failed to set owner references for %s", obj.GetName()))
		return
	}

	logger.Info(fmt.Sprintf("apiversion: %v, kind: %v, name: %v, uid: %v", owner.APIVersion, owner.Kind, owner.Name, owner.UID))

	obj.SetOwnerReferences([]metav1.OwnerReference{
		{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       owner.Name,
			UID:        owner.UID,
		},
	})
}

func setWebhookOwnerReferences(c client.Client, logger logr.Logger, obj metav1.Object) {
	channelCrdName := "channels.apps.open-cluster-management.io"
	key := types.NamespacedName{Name: channelCrdName}
	owner := &apixv1.CustomResourceDefinition{}

	if err := c.Get(context.TODO(), key, owner); err != nil {
		logger.Error(err, fmt.Sprintf("Failed to set webhook owner references for %s", obj.GetName()))
		return
	}

	// The TypeMeta fields (APIVersion and Kind) are not automatically populated
	// when you fetch a Kubernetes resource using the client-go or controller-runtime client
	owner.APIVersion = "apiextensions.k8s.io/v1"
	owner.Kind = "CustomResourceDefinition"

	obj.SetOwnerReferences([]metav1.OwnerReference{
		{
			APIVersion: owner.APIVersion,
			Kind:       owner.Kind,
			Name:       owner.Name,
			UID:        owner.UID,
		},
	})
}

func newWebhookServiceTemplate(isExternalAPIServer bool, svcKey types.NamespacedName, webHookPort,
	webHookServicePort int, deploymentSelector map[string]string) *corev1.Service {
	if isExternalAPIServer {
		// if the service is created on hosted cluster, no selector and target port should be specicified as the webhook server pod is not running there
		return &corev1.Service{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Service",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      svcKey.Name,
				Namespace: svcKey.Namespace,
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Port: int32(webHookServicePort),
					},
				},
			},
		}
	}

	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcKey.Name,
			Namespace: svcKey.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port:       int32(webHookServicePort),
					TargetPort: intstr.FromInt(webHookPort),
				},
			},
			Selector: deploymentSelector,
		},
	}
}

func newWebhookEndpointTemplate(svcKey types.NamespacedName, webHookServicePort int, serviceClusterIP string) *corev1.Endpoints {
	return &corev1.Endpoints{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Endpoints",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcKey.Name,
			Namespace: svcKey.Namespace,
		},
		Subsets: []corev1.EndpointSubset{
			{
				Addresses: []corev1.EndpointAddress{
					{
						IP: serviceClusterIP,
					},
				},
				Ports: []corev1.EndpointPort{
					{
						Port: int32(webHookServicePort),
					},
				},
			},
		},
	}
}

func newValidatingWebhookCfg(svcKey types.NamespacedName, validatorName, path string, ca []byte,
	gvk schema.GroupVersionKind, ops []admissionv1.OperationType) *admissionv1.ValidatingWebhookConfiguration {
	ignore := admissionv1.Ignore
	side := admissionv1.SideEffectClassNone
	timeoutSeconds := int32(30)

	return &admissionv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: validatorName,
		},

		Webhooks: []admissionv1.ValidatingWebhook{{
			Name:                    validatorName,
			AdmissionReviewVersions: []string{"v1beta1"},
			SideEffects:             &side,
			FailurePolicy:           &ignore,
			ClientConfig: admissionv1.WebhookClientConfig{
				Service: &admissionv1.ServiceReference{
					Name:      svcKey.Name,
					Namespace: svcKey.Namespace,
					Path:      &path,
				},
				CABundle: ca,
			},
			Rules: []admissionv1.RuleWithOperations{
				{
					Rule: admissionv1.Rule{
						APIGroups:   []string{gvk.Group},
						APIVersions: []string{gvk.Version},
						Resources:   []string{gvk.Kind},
					},
					Operations: ops,
				},
			},
			TimeoutSeconds: &timeoutSeconds,
		},
		},
	}
}
