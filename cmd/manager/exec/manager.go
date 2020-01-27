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
	"runtime"

	"k8s.io/client-go/kubernetes"
	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"

	"github.com/prometheus/common/log"

	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	kubemetrics "github.com/operator-framework/operator-sdk/pkg/kube-metrics"
	"github.com/operator-framework/operator-sdk/pkg/leader"
	"github.com/operator-framework/operator-sdk/pkg/metrics"
	"github.com/operator-framework/operator-sdk/pkg/restmapper"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"k8s.io/klog"

	"github.com/IBM/multicloud-operators-channel/pkg/apis"
	"github.com/IBM/multicloud-operators-channel/pkg/controller"
	"github.com/IBM/multicloud-operators-channel/pkg/utils"
	"github.com/IBM/multicloud-operators-channel/pkg/webhook"

	gitsync "github.com/IBM/multicloud-operators-channel/pkg/synchronizer/githubsynchronizer"
	helmsync "github.com/IBM/multicloud-operators-channel/pkg/synchronizer/helmreposynchronizer"
	objsync "github.com/IBM/multicloud-operators-channel/pkg/synchronizer/objectstoresynchronizer"
)

// Change below variables to serve metrics on different host or port.
var (
	metricsHost               = "0.0.0.0"
	metricsPort         int32 = 8383
	operatorMetricsPort int32 = 8686
)

const exitCode = 1

func printVersion() {
	klog.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	klog.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	klog.Info(fmt.Sprintf("Version of operator-sdk: %v", sdkVersion.Version))
}

//RunManager initial controller, synchronizer and start manager
func RunManager(sig <-chan struct{}) {
	printVersion()

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		klog.Error(err, "")
		os.Exit(exitCode)
	}

	ctx := context.TODO()
	// Become the leader before proceeding
	err = leader.Become(ctx, "multicloud-operators-channel-lock")
	if err != nil {
		klog.Error(err, "")
		os.Exit(exitCode)
	}

	// Create a new Cmd to provide shared dependencies and start components
	mgr, err := manager.New(cfg, manager.Options{
		MapperProvider:     restmapper.NewDynamicRESTMapper,
		MetricsBindAddress: fmt.Sprintf("%s:%d", metricsHost, metricsPort),
	})
	if err != nil {
		klog.Error(err, "")
		os.Exit(exitCode)
	}

	// Create channel descriptor
	chdesc, err := utils.CreateObjectStorageChannelDescriptor()
	if err != nil {
		klog.Error("unable to create channel descriptor.", err)
		os.Exit(exitCode)
	}

	// Create channel synchronizer
	osync, err := objsync.CreateObjectStoreSynchronizer(cfg, chdesc, options.SyncInterval)

	if err != nil {
		klog.Error("unable to create object-store syncrhonizer on destination cluster.", err)
		os.Exit(exitCode)
	}

	err = mgr.Add(osync)
	if err != nil {
		klog.Error("Failed to register synchronizer.", err)
		os.Exit(exitCode)
	}

	// Create channel synchronizer for helm repo
	hsync, err := helmsync.CreateHelmrepoSynchronizer(cfg, mgr.GetScheme(), options.SyncInterval)

	if err != nil {
		klog.Error("unable to create helo-repo syncrhonizer on destination cluster.", err)
		os.Exit(exitCode)
	}

	err = mgr.Add(hsync)
	if err != nil {
		klog.Error("Failed to register synchronizer.", err)
		os.Exit(exitCode)
	}

	// Create channel synchronizer for github
	gsync, err := gitsync.CreateGithubSynchronizer(cfg, mgr.GetScheme(), options.SyncInterval)

	if err != nil {
		klog.Error("unable to create github syncrhonizer on destination cluster.", err)
		os.Exit(exitCode)
	}

	err = mgr.Add(gsync)
	if err != nil {
		klog.Error("Failed to register synchronizer.", err)
		os.Exit(exitCode)
	}

	klog.Info("Registering Components.")

	// Setup Scheme for all resources
	klog.Info("setting up scheme")

	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		klog.Error(err, "unable add APIs to scheme")
		os.Exit(exitCode)
	}

	//create channel events handler on hub cluster.
	hubClientSet, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Error("Failed to get hub cluster clientset. err: ", err)
		os.Exit(exitCode)
	}

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: hubClientSet.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(mgr.GetScheme(), v1.EventSource{Component: "channel"})

	// Setup all Controllers
	klog.Info("Setting up controller")

	if err := controller.AddToManager(mgr, recorder, chdesc, hsync, gsync); err != nil {
		klog.Error(err, "unable to register controllers to the manager")
		os.Exit(exitCode)
	}

	klog.Info("setting up webhooks")

	if err := webhook.AddToManager(mgr); err != nil {
		klog.Error(err, "unable to register webhooks to the manager")
		os.Exit(exitCode)
	}

	if err = serveCRMetrics(cfg); err != nil {
		klog.Info("Could not generate and serve custom resource metrics", "error", err.Error())
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
		klog.Info("Could not create metrics Service", "error", err.Error())
	}

	// CreateServiceMonitors will automatically create the prometheus-operator ServiceMonitor resources
	// necessary to configure Prometheus to scrape metrics from this operator.
	services := []*v1.Service{service}
	_, err = metrics.CreateServiceMonitors(cfg, "", services)

	if err != nil {
		log.Info("Could not create ServiceMonitor object", "error", err.Error())
		// If this operator is deployed to a cluster without the prometheus-operator running, it will return
		// ErrServiceMonitorNotPresent, which can be used to safely skip ServiceMonitor creation.
		if err == metrics.ErrServiceMonitorNotPresent {
			klog.Info("Install prometheus-operator in your cluster to create ServiceMonitor objects", "error", err.Error())
		}
	}

	klog.Info("Starting the Cmd.")

	// Start the Cmd
	if err := mgr.Start(sig); err != nil {
		klog.Error(err, "Manager exited non-zero")
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
