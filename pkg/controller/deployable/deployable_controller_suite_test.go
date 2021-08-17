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

package deployable

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/go-logr/zapr"
	"go.uber.org/zap/zapcore"

	"github.com/onsi/ginkgo/config"
	"github.com/onsi/ginkgo/reporters"
	"github.com/onsi/ginkgo/reporters/stenographer"
	"github.com/onsi/gomega/gexec"
	uzap "go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/open-cluster-management/multicloud-operators-channel/pkg/apis"
)

const StartTimeout = 60 // seconds
var testEnv *envtest.Environment
var recFn reconcile.Reconciler
var requests chan reconcile.Request
var cCfg *rest.Config

func TestChannelDeployableReconcile(t *testing.T) {
	RegisterFailHandler(Fail)

	noColor := true
	gCfg := config.DefaultReporterConfigType{NoColor: noColor, Verbose: true}
	//make sure the debug log is printed to he test env
	rep := reporters.NewDefaultReporter(gCfg, stenographer.New(!noColor, false, os.Stdout))
	RunSpecsWithCustomReporters(t,
		"Deployable Controller Suite", []Reporter{rep})
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
			CRDDirectoryPaths: []string{filepath.Join("..", "..", "..", "deploy", "crds"), filepath.Join("..", "..", "..", "deploy", "dependent-crds")},
		}
	}

	var err error
	// be careful, if we use shorthand assignment, the the cCfg will be a local variable
	cCfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cCfg).ToNot(BeNil())

	//initialize the logger for test suit use
	//zapLog, err := uzap.NewDevelopment()
	//zapLog, err := uzap.NewProduction()
	var c client.Client
	if c, err = client.New(cCfg, client.Options{Scheme: scheme.Scheme}); err != nil {
		log.Fatal(err)
	}

	logConfig := uzap.NewProductionConfig()
	logConfig.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	zapLog, err := logConfig.Build()

	Expect(err).ToNot(HaveOccurred())
	ctrl.SetLogger(zapr.NewLogger(zapLog))

	err = apis.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = c.Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "ch-ns-2"},
	})
	if err != nil {
		log.Fatal(err)
	}

	err = c.Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "dpl-ns-2"},
	})
	if err != nil {
		log.Fatal(err)
	}

	close(done)
}, StartTimeout)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	gexec.KillAndWait(5 * time.Second)
	Expect(testEnv.Stop()).ToNot(HaveOccurred())
})

// SetupTestReconcile returns a reconcile.Reconcile implementation that delegates to inner and
// writes the request to requests after Reconcile is finished.
func SetupTestReconcile(inner reconcile.Reconciler) (reconcile.Reconciler, chan reconcile.Request) {
	requests := make(chan reconcile.Request)
	fn := reconcile.Func(func(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
		result, err := inner.Reconcile(ctx, req)
		requests <- req
		return result, err
	})

	return fn, requests
}
