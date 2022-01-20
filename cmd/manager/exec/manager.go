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
	"log"
	"os"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/go-logr/zapr"
	uzap "go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	admissionv1 "k8s.io/api/admissionregistration/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	chv1 "github.com/stolostron/multicloud-operators-channel/pkg/apis/apps/v1"

	"github.com/stolostron/multicloud-operators-channel/pkg/apis"
	"github.com/stolostron/multicloud-operators-channel/pkg/controller"
	"github.com/stolostron/multicloud-operators-channel/pkg/utils"
	chWebhook "github.com/stolostron/multicloud-operators-channel/pkg/webhook"

	placementutils "github.com/open-cluster-management/multicloud-operators-placementrule/pkg/utils"
	helmsync "github.com/stolostron/multicloud-operators-channel/pkg/synchronizer/helmreposynchronizer"
	objsync "github.com/stolostron/multicloud-operators-channel/pkg/synchronizer/objectstoresynchronizer"
)

// Change below variables to serve metrics on different host or port.
var (
	metricsHost             = "0.0.0.0"
	metricsPort         int = 8384
	operatorMetricsPort int = 8687
)

const (
	exitCode = 1
	kindName = "channels"
)

//RunManager initial controller, synchronizer and start manager
func RunManager() {
	var err error

	var zapLog *uzap.Logger

	logConfig := uzap.NewProductionConfig()

	//making sure the time format in production is human readable
	logConfig.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	zapLog, err = logConfig.Build()

	if options.LogLevel {
		zapLog, err = uzap.NewDevelopment()
	}

	if err != nil {
		log.Fatal(err)
		os.Exit(exitCode)
	}

	logf.SetLogger(zapr.NewLogger(zapLog))

	logger := logf.Log.WithName("set up manager")

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		logger.Error(err, "")
		os.Exit(exitCode)
	}

	enableLeaderElection := false

	if _, err := rest.InClusterConfig(); err == nil {
		logger.Info("LeaderElection enabled as running in a cluster")

		enableLeaderElection = true
	} else {
		logger.Info("LeaderElection disabled as not running in a cluster")
	}

	// Create a new Cmd to provide shared dependencies and start components
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		MetricsBindAddress:      fmt.Sprintf("%s:%d", metricsHost, metricsPort),
		Port:                    operatorMetricsPort,
		LeaderElection:          enableLeaderElection,
		LeaderElectionID:        "multicloud-operators-channel-leader.open-cluster-management.io",
		LeaderElectionNamespace: "kube-system",
	})

	if err != nil {
		logger.Error(err, "failed to start manager")
		os.Exit(1)
	}

	// Create dynamic client
	dynamicClient := dynamic.NewForConfigOrDie(ctrl.GetConfigOrDie())

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
	eventBroadcaster.StartLogging(zapLog.Sugar().Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: hubClientSet.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(mgr.GetScheme(), v1.EventSource{Component: "channel"})

	// Setup all Controllers
	logger.Info("Setting up controller")

	if err := controller.AddToManager(mgr, dynamicClient, recorder, logf.Log.WithName("controllers"), chdesc, hsync); err != nil {
		logger.Error(err, "unable to register controllers to the manager")
		os.Exit(exitCode)
	}

	sig := signals.SetupSignalHandler()

	logger.Info("Detecting ACM cluster API service...")
	placementutils.DetectClusterRegistry(mgr.GetAPIReader(), sig)

	// Setup webhooks
	if !options.Debug {
		logger.Info("setting up webhook server")

		wiredWebhook, err := chWebhook.NewWireUp(mgr, sig, chWebhook.ValidateLogic)
		if err != nil {
			logger.Error(err, "failed to initial wire up webhook")
			os.Exit(exitCode)
		}

		clt, err := client.New(ctrl.GetConfigOrDie(), client.Options{})
		if err != nil {
			logger.Error(err, "failed to create a client for webhook to get CA cert secret")
			os.Exit(exitCode)
		}

		caCert, err := wiredWebhook.Attach(clt)
		if err != nil {
			logger.Error(err, "failed to attach webhook")
			os.Exit(exitCode)
		}

		go func() {
			if err := wiredWebhook.WireUpWebhookSupplymentryResource(caCert, chv1.SchemeGroupVersion.WithKind(kindName),
				[]admissionv1.OperationType{admissionv1.Create}, chWebhook.DelPreValiationCfg20); err != nil {
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
