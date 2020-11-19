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

package exec

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"

	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	kubemetrics "github.com/operator-framework/operator-sdk/pkg/kube-metrics"
	"github.com/operator-framework/operator-sdk/pkg/leader"

	"github.com/operator-framework/operator-sdk/pkg/metrics"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	admissionv1 "k8s.io/api/admissionregistration/v1"

	logf "sigs.k8s.io/controller-runtime/pkg/log"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"

	"github.com/open-cluster-management/multicloud-operators-channel/pkg/apis"
	"github.com/open-cluster-management/multicloud-operators-channel/pkg/controller"
	"github.com/open-cluster-management/multicloud-operators-channel/pkg/log/zap"
	"github.com/open-cluster-management/multicloud-operators-channel/pkg/utils"
	chWebhook "github.com/open-cluster-management/multicloud-operators-channel/pkg/webhook"

	helmsync "github.com/open-cluster-management/multicloud-operators-channel/pkg/synchronizer/helmreposynchronizer"
	objsync "github.com/open-cluster-management/multicloud-operators-channel/pkg/synchronizer/objectstoresynchronizer"
	placementutils "github.com/open-cluster-management/multicloud-operators-placementrule/pkg/utils"
)

// Change below variables to serve metrics on different host or port.
var (
	metricsHost               = "0.0.0.0"
	metricsPort         int32 = 8384
	operatorMetricsPort int32 = 8687
	setupLog                  = logf.Log.WithName("setup")
)

const (
	exitCode = 1
	kindName = "channels"
)

func printVersion() {
	setupLog.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	setupLog.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	setupLog.Info(fmt.Sprintf("Version of operator-sdk: %v", sdkVersion.Version))
}

//RunManager initial controller, synchronizer and start manager
func RunManager(sig <-chan struct{}) {
	logf.SetLogger(zap.Logger())

	printVersion()

	logger := logf.Log
	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		logger.Error(err, "")
		os.Exit(exitCode)
	}

	ctx := context.TODO()
	// Become the leader before proceeding
	err = leader.Become(ctx, "multicloud-operators-channel-lock")
	if err != nil {
		logger.Error(err, "")
		os.Exit(exitCode)
	}

	// Create a new Cmd to provide shared dependencies and start components
	mgr, err := manager.New(cfg, manager.Options{MetricsBindAddress: fmt.Sprintf("%s:%d", metricsHost, metricsPort)})
	if err != nil {
		logger.Error(err, "")
		os.Exit(exitCode)
	}

	// Create channel descriptor is user for the object bucket
	chdesc, err := utils.CreateObjectStorageChannelDescriptor()
	if err != nil {
		logger.Error(err, "unable to create channel descriptor.")
		os.Exit(exitCode)
	}

	//Create channel synchronizer
	osync, err := objsync.CreateObjectStoreSynchronizer(cfg, chdesc, options.SyncInterval)

	if err != nil {
		logger.Error(err, "unable to create object-store syncrhonizer on destination cluster.")
		os.Exit(exitCode)
	}

	err = mgr.Add(osync)
	if err != nil {
		logger.Error(err, "Failed to register synchronizer.")
		os.Exit(exitCode)
	}

	// Create channel synchronizer for helm repo
	hsync, err := helmsync.CreateHelmrepoSynchronizer(cfg, mgr.GetScheme(), options.SyncInterval)

	if err != nil {
		logger.Error(err, "unable to create helo-repo syncrhonizer on destination cluster.")
		os.Exit(exitCode)
	}

	err = mgr.Add(hsync)
	if err != nil {
		logger.Error(err, "Failed to register synchronizer.")
		os.Exit(exitCode)
	}

	logger.Info("Registering Components.")

	// Setup Scheme for all resources
	logger.Info("setting up scheme")

	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		logger.Error(err, "unable add APIs to scheme")
		os.Exit(exitCode)
	}

	//create channel events handler on hub cluster.
	hubClientSet, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		logger.Error(err, "Failed to get hub cluster clientset.")
		os.Exit(exitCode)
	}

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: hubClientSet.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(mgr.GetScheme(), v1.EventSource{Component: "channel"})

	// Setup all Controllers
	logger.Info("Setting up controller")

	if err := controller.AddToManager(mgr, recorder, logger.WithName("controllers"), chdesc, hsync); err != nil {
		logger.Error(err, "unable to register controllers to the manager")
		os.Exit(exitCode)
	}

	if err = serveCRMetrics(cfg); err != nil {
		logger.Info("Could not generate and serve custom resource metrics", "error", err.Error())
	}

	// Add to the below struct any other metrics ports you want to expose.
	servicePorts := []v1.ServicePort{
		{
			Port:       metricsPort,
			Name:       metrics.OperatorPortName,
			Protocol:   v1.ProtocolTCP,
			TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: metricsPort},
		},
		{
			Port:       operatorMetricsPort,
			Name:       metrics.CRPortName,
			Protocol:   v1.ProtocolTCP,
			TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: operatorMetricsPort},
		},
	}
	// Create Service object to expose the metrics port(s).
	service, err := metrics.CreateMetricsService(ctx, cfg, servicePorts)
	if err != nil {
		logger.Info("Could not create metrics Service", "error", err.Error())
	}

	// CreateServiceMonitors will automatically create the prometheus-operator ServiceMonitor resources
	// necessary to configure Prometheus to scrape metrics from this operator.
	services := []*v1.Service{service}
	_, err = metrics.CreateServiceMonitors(cfg, "", services)

	if err != nil {
		logger.Info("Could not create ServiceMonitor object", "error", err.Error())
		// If this operator is deployed to a cluster without the prometheus-operator running, it will return
		// ErrServiceMonitorNotPresent, which can be used to safely skip ServiceMonitor creation.
		if err == metrics.ErrServiceMonitorNotPresent {
			logger.Info("Install prometheus-operator in your cluster to create ServiceMonitor objects", "error", err.Error())
		}
	}

	logger.Info("Detecting ACM cluster API service...")
	placementutils.DetectClusterRegistry(mgr.GetAPIReader(), sig)

	// Setup webhooks
	if options.Debug {
		logger.Info("setting up webhook server")

		wbhCertDir := func(w *chWebhook.WireUp) {
			w.CertDir = filepath.Join(os.TempDir(), "k8s-webhook-server", "serving-certs")
		}

		wbhLogger := func(w *chWebhook.WireUp) {
			w.Logger = logger.WithName("channel-operator-duplicate-webhook")
		}

		wiredWebhook, err := chWebhook.NewWireUp(mgr, sig, wbhCertDir, wbhLogger, chWebhook.ValidateLogic)
		if err != nil {
			logger.Error(err, "failed to initial wire up webhook")
			os.Exit(exitCode)
		}

		caCert, err := wiredWebhook.Attach()
		if err != nil {
			logger.Error(err, "failed to wire up webhook")
			os.Exit(exitCode)
		}

		go func() {
			if err := wiredWebhook.WireUpWebhookSupplymentryResource(caCert,
				chv1.SchemeGroupVersion.WithKind(kindName), []admissionv1.OperationType{admissionv1.Create}, chWebhook.DelPreValiationCfg20); err != nil {
				logger.Error(err, "failed to set up webhook configuration")
				os.Exit(exitCode)
			}
		}()
	}

	logger.Info("Starting the Cmd.")
	// Start the Cmd
	if err := mgr.Start(sig); err != nil {
		logger.Error(err, "Manager exited non-zero")
		os.Exit(exitCode)
	}
}

// serveCRMetrics gets the Operator/CustomResource GVKs and generates metrics based on those types.
// It serves those metrics on "http://metricsHost:operatorMetricsPort".
func serveCRMetrics(cfg *rest.Config) error {
	// Below function returns filtered operator/CustomResource specific GVKs.
	// For more control override the below GVK list with your own custom logic.
	filteredGVK, err := k8sutil.GetGVKsFromAddToScheme(apis.AddToScheme)
	if err != nil {
		return err
	}
	// Get the namespace the operator is currently deployed in.
	operatorNs, err := k8sutil.GetOperatorNamespace()
	if err != nil {
		return err
	}
	// To generate metrics in other namespaces, add the values below.
	ns := []string{operatorNs}
	// Generate and serve custom resource specific metrics.
	return kubemetrics.GenerateAndServeCRMetrics(cfg, ns, filteredGVK, metricsHost, operatorMetricsPort)
}
