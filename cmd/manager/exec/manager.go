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
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"

	v1 "k8s.io/api/core/v1"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	admissionv1 "k8s.io/api/admissionregistration/v1"

	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

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
	metricsHost             = "0.0.0.0"
	metricsPort         int = 8384
	operatorMetricsPort int = 8687
	setupLog                = logf.Log.WithName("setup")
)

const (
	exitCode = 1
	kindName = "channels"
)

//RunManager initial controller, synchronizer and start manager
func RunManager() {
	logf.SetLogger(zap.Logger())

	logger := logf.Log
	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		logger.Error(err, "")
		os.Exit(exitCode)
	}

	enableLeaderElection := false
	if _, err := rest.InClusterConfig(); err == nil {
		klog.Info("LeaderElection enabled as running in a cluster")
		enableLeaderElection = true
	} else {
		klog.Info("LeaderElection disabled as not running in a cluster")
	}

	// Create a new Cmd to provide shared dependencies and start components
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		MetricsBindAddress:      fmt.Sprintf("%s:%d", metricsHost, metricsPort),
		Port:                    operatorMetricsPort,
		LeaderElection:          enableLeaderElection,
		LeaderElectionID:        "multicloud-operators-placementrule-leader.open-cluster-management.io",
		LeaderElectionNamespace: "kube-system",
	})

	if err != nil {
		klog.Error(err, "")
		os.Exit(1)
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

	sig := signals.SetupSignalHandler()

	logger.Info("Detecting ACM cluster API service...")
	placementutils.DetectClusterRegistry(mgr.GetAPIReader(), sig)

	// Setup webhooks
	if !options.Debug {
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
