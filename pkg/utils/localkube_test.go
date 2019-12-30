// Licensed Materials - Property of IBM
// (c) Copyright IBM Corporation 2016, 2019. All Rights Reserved.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP  Schedule Contract with IBM Corp.

package utils_test

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/IBM/multicloud-operators-channel/pkg/apis"
	appv1alpha1 "github.com/IBM/multicloud-operators-channel/pkg/apis/app/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var cfg *rest.Config
var c client.Client

var ObjTobeDeleted = []func(){}
var ObjTobeCreated = []runtime.Object{}

var chName = "qa"
var chNs = "ch-qa"
var chType = "namespace"

var dplName = "orphandpl"
var dplNs = "default"

var key = types.NamespacedName{
	Name:      chName,
	Namespace: chNs,
}

var chObj = &appv1alpha1.Channel{
	TypeMeta: metav1.TypeMeta{
		Kind:       "app.ibm.com",
		APIVersion: "v1alpha1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:      chName,
		Namespace: chNs,
	},
	Spec: appv1alpha1.ChannelSpec{
		Type: appv1alpha1.ChannelType("namespace"),
	},
}

// testing.M is going to set up a local k8s env and provide the client, so the other test case can access to the cluster
func TestMain(m *testing.M) {
	log.Printf("Enter TestMain\n")

	channelCrdPath := filepath.Join("..", "..", "config", "crds")
	dplCrdPath := filepath.Join("..", "..", "vendor", "github.ibm.com", "IBMMulticloudPlatform", "deployable", "config", "crds")

	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			channelCrdPath, dplCrdPath},
	}
	s := scheme.Scheme

	apis.AddToScheme(s)

	appv1alpha1.SchemeBuilder.AddToScheme(s)
	var err error
	if cfg, err = testEnv.Start(); err != nil {
		log.Fatal(err)
	}

	// mgr, _ := manager.New(cfg, manager.Options{Scheme: s})
	// c = mgr.GetClient()
	if c, err = client.New(cfg, client.Options{Scheme: s}); err != nil {
		log.Fatal(err)
	}
	ObjTobeCreated = append(ObjTobeCreated, chObj)

	InitObjQueue()

	for _, obj := range ObjTobeCreated {
		closeFunc, err := GenerateCRsAtLocalKube(c, obj)

		if err != nil {
			log.Fatalf("encounter error will process the ObjTobeCreated queue, error is %v, obj %v", err, obj)
		}
		if closeFunc != nil {
			ObjTobeDeleted = append(ObjTobeDeleted, closeFunc)
		}
	}
	code := m.Run()

	for _, f := range ObjTobeDeleted {
		if f != nil {
			f()
		}
	}

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
