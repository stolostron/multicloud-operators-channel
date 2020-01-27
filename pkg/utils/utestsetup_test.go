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

package utils_test

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/IBM/multicloud-operators-channel/pkg/apis"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	appv1alpha1 "github.com/IBM/multicloud-operators-channel/pkg/apis/app/v1alpha1"
)

var cfg *rest.Config
var c client.Client

// testing.M is going to set up a local k8s env and provide the client, so the other test case can access to the cluster
func TestMain(m *testing.M) {
	log.Printf("Enter TestMain\n")

	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "deploy", "crds")},
	}

	s := scheme.Scheme

	apis.AddToScheme(s)

	appv1alpha1.SchemeBuilder.AddToScheme(s)

	var err error

	if cfg, err = testEnv.Start(); err != nil {
		log.Fatal(err)
	}

	if c, err = client.New(cfg, client.Options{Scheme: s}); err != nil {
		log.Fatal(err)
	}

	code := m.Run()

	testEnv.Stop()
	log.Printf("Exiting TestMain\n")
	os.Exit(code)
}

// Will generate and CR and provide a delete func of it
func GenerateCRsAtLocalKube(c client.Client, instance runtime.Object) (func(), error) {
	err := c.Create(context.TODO(), instance)
	if err != nil {
		log.Printf("Can't create %#v at the local Kube due to: %v", instance.GetObjectKind(), err)
		return nil, err
	}

	closeFunc := func() {
		err := c.Delete(context.TODO(), instance)
		if err != nil {
			log.Fatalf("failed to delete %v due to error: %v", instance.GetObjectKind(), err)
		}
	}

	return closeFunc, nil
}
