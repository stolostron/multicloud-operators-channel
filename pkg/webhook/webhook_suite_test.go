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
	"log"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/zapr"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	"github.com/onsi/ginkgo/reporters"
	"github.com/onsi/ginkgo/reporters/stenographer"
	. "github.com/onsi/gomega"

	"github.com/onsi/gomega/gexec"
	uzap "go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	mgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"open-cluster-management.io/multicloud-operators-channel/pkg/apis"
	chv1 "open-cluster-management.io/multicloud-operators-channel/pkg/apis/apps/v1"
)

const (
	StartTimeout = 60 // seconds
)

var testEnv *envtest.Environment
var k8sManager mgr.Manager
var k8sClient client.Client
var cfg *rest.Config

var (
	webhookValidatorName = "test-suite-webhook"
	validatorPath        = "/v1-validate"
	webhookName          = "channels.apps.open-cluster-management.webhook"
	resourceName         = "channels"
	stop                 = ctrl.SetupSignalHandler()
)

func TestMain(m *testing.M) {
	t := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "deploy", "crds"),
			filepath.Join("..", "..", "..", "hack", "test"),
		},
	}

	apis.AddToScheme(scheme.Scheme)

	var err error
	if cfg, err = t.Start(); err != nil {
		log.Fatal(err)
	}

	if k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme}); err != nil {
		log.Fatal(err)
	}

	err = k8sClient.Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
	})
	if err != nil {
		log.Fatal(err)
	}

	code := m.Run()

	t.Stop()
	os.Exit(code)
}

// StartTestManager adds recFn
func StartTestManager(ctx context.Context, mgr mgr.Manager, g *GomegaWithT) *sync.WaitGroup {
	wg := &sync.WaitGroup{}
	wg.Add(1)

	go func() {
		wg.Done()
		mgr.Start(ctx)
	}()

	return wg
}

func TestChannelWebhook(t *testing.T) {
	RegisterFailHandler(Fail)

	noColor := true
	gCfg := config.DefaultReporterConfigType{NoColor: noColor, Verbose: true}

	rep := reporters.NewDefaultReporter(gCfg, stenographer.New(!noColor, false, os.Stdout))
	RunSpecsWithCustomReporters(t,
		"Channel webhook Suite", []Reporter{rep})
}

var _ = BeforeSuite(func(done Done) {
	By("bootstrapping test environment")

	t := true
	if os.Getenv("TEST_USE_EXISTING_CLUSTER") == "true" {
		testEnv = &envtest.Environment{
			UseExistingCluster: &t,
		}
	} else {
		testEnv = &envtest.Environment{
			CRDDirectoryPaths: []string{filepath.Join("..", "..", "deploy", "crds"),
				filepath.Join("..", "..", "deploy", "dependent-crds")},
		}
	}

	var err error
	// be careful, if we use shorthand assignment, the cCfg will be a local variable
	initializeWebhookInEnvironment()
	cfg, err := testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = apis.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sManager, err = mgr.New(cfg, mgr.Options{
		MetricsBindAddress: "0",
		Port:               testEnv.WebhookInstallOptions.LocalServingPort,
		Host:               testEnv.WebhookInstallOptions.LocalServingHost,
		CertDir:            testEnv.WebhookInstallOptions.LocalServingCertDir,
	})

	Expect(err).NotTo(HaveOccurred())

	zapLog, err := uzap.NewDevelopment()

	//zapLog, err := uzap.NewProduction()
	Expect(err).ToNot(HaveOccurred())

	ctrl.SetLogger(zapr.NewLogger(zapLog))

	hookServer := k8sManager.GetWebhookServer()

	k8sClient, err = client.New(testEnv.Config, client.Options{})
	Expect(err).NotTo(HaveOccurred())

	err = k8sClient.Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
	})
	Expect(err).ToNot(HaveOccurred())

	err = k8sClient.Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "test-ns"},
	})
	Expect(err).ToNot(HaveOccurred())

	hookServer.Register(validatorPath,
		&webhook.Admission{
			Handler: &ChannelValidator{
				Client: k8sClient,
				Logger: ctrl.Log,
			}})
	Expect(err).ToNot(HaveOccurred())

	go func() {
		Expect(hookServer.Start(stop)).Should(Succeed())
	}()

	close(done)
}, StartTimeout)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	gexec.KillAndWait(5 * time.Second)
	Expect(testEnv.Stop()).ToNot(HaveOccurred())
})

func initializeWebhookInEnvironment() {
	namespacedScopeV1 := admissionv1.NamespacedScope
	failedTypeV1 := admissionv1.Fail
	equivalentTypeV1 := admissionv1.Equivalent
	noSideEffectsV1 := admissionv1.SideEffectClassNone
	webhookPathV1 := validatorPath

	vwc := []*admissionv1.ValidatingWebhookConfiguration{}
	vwc = append(vwc, &admissionv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: webhookValidatorName,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "ValidatingWebhookConfiguration",
			APIVersion: "admissionregistration.k8s.io/v1beta1",
		},
		Webhooks: []admissionv1.ValidatingWebhook{
			{
				Name:                    webhookName,
				AdmissionReviewVersions: []string{"v1beta1"},
				Rules: []admissionv1.RuleWithOperations{
					{
						Operations: []admissionv1.OperationType{"CREATE", "UPDATE"},
						Rule: admissionv1.Rule{
							APIGroups:   []string{chv1.SchemeGroupVersion.Group},
							APIVersions: []string{chv1.SchemeGroupVersion.Version},
							Resources:   []string{resourceName},
							Scope:       &namespacedScopeV1,
						},
					},
				},
				FailurePolicy: &failedTypeV1,
				MatchPolicy:   &equivalentTypeV1,
				SideEffects:   &noSideEffectsV1,
				ClientConfig: admissionv1.WebhookClientConfig{
					Service: &admissionv1.ServiceReference{
						Name:      "channel-validation-service",
						Namespace: "default",
						Path:      &webhookPathV1,
					},
				},
			},
		},
	})

	testEnv.WebhookInstallOptions = envtest.WebhookInstallOptions{
		ValidatingWebhooks: vwc,
	}
}
