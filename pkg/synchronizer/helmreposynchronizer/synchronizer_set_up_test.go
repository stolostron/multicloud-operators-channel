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

package helmreposynchronizer

import (
	"log"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/onsi/gomega"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"
	"github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis"
)

var cfg *rest.Config
var c client.Client

// testing.M is going to set up a local k8s env and provide the client, so the other test case can access to the cluster
func TestMain(m *testing.M) {
	customAPIServerFlags := []string{"--disable-admission-plugins=NamespaceLifecycle,LimitRanger,ServiceAccount," +
		"TaintNodesByCondition,Priority,DefaultTolerationSeconds,DefaultStorageClass,StorageObjectInUseProtection," +
		"PersistentVolumeClaimResize,ResourceQuota",
	}

	apiServerFlags := append([]string(nil), envtest.DefaultKubeAPIServerFlags...)
	apiServerFlags = append(apiServerFlags, customAPIServerFlags...)

	testEnv := &envtest.Environment{
		CRDDirectoryPaths:  []string{filepath.Join("..", "..", "..", "deploy", "crds"), filepath.Join("..", "..", "..", "deploy", "dependent-crds")},
		KubeAPIServerFlags: apiServerFlags,
	}

	s := scheme.Scheme

	apis.AddToScheme(s)

	chv1.SchemeBuilder.AddToScheme(s)

	var err error

	if cfg, err = testEnv.Start(); err != nil {
		log.Fatal(err)
	}

	if c, err = client.New(cfg, client.Options{Scheme: s}); err != nil {
		log.Fatal(err)
	}

	code := m.Run()

	testEnv.Stop()
	os.Exit(code)
}

// StartTestManager adds recFn
func StartTestManager(mgr manager.Manager, g *gomega.GomegaWithT) (chan struct{}, *sync.WaitGroup) {
	stop := make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()
		g.Expect(mgr.Start(stop)).NotTo(gomega.HaveOccurred())
	}()

	return stop, wg
}
