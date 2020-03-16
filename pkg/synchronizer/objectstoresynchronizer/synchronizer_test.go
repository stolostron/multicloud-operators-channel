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

package objectstoresynchronizer

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"
	"github.com/open-cluster-management/multicloud-operators-channel/pkg/utils"
	dplv1 "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis/apps/v1"
)

//1. objectstore is empty, delete all deployables, if there's any
// Test_alginClusterResourceWithHost_emptyHost
//2. if objectstore has entry, then it should be deployed
// Test_alginClusterResourceWithHost_createDplBasedOnHost
//3. some deployables changed at objectstore then the local deployables should be updated as well
// Test_alginClusterResourceWithHost_updateDplBasedOnHost
func Test_syncChannelsWithObjStore(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(gomega.HaveOccurred())

	g.Expect(err).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	chdesc, _ := utils.CreateObjectStorageChannelDescriptor()

	objsync, _ := CreateObjectStoreSynchronizer(mgr.GetConfig(), chdesc, 2)

	g.Expect(objsync.syncChannelsWithObjStore()).ShouldNot(gomega.HaveOccurred())
}

//empty host
func Test_alginClusterResourceWithHost_emptyHost(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(gomega.HaveOccurred())

	g.Expect(err).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	c := mgr.GetClient()

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	tKey := types.NamespacedName{Name: "tch", Namespace: "tns"}

	srtName := "srt-ref"
	refSrt := &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      srtName,
			Namespace: tKey.Namespace,
		},
		Data: map[string][]byte{
			utils.SecretMapKeyAccessKeyID:     {},
			utils.SecretMapKeySecretAccessKey: {},
		},
	}

	tCh := &chv1.Channel{
		ObjectMeta: v1.ObjectMeta{
			Name:      tKey.Name,
			Namespace: tKey.Namespace,
		},
		Spec: chv1.ChannelSpec{
			Type:     chv1.ChannelType("objectbucket"),
			Pathname: "/" + tKey.Namespace,
			SecretRef: &corev1.ObjectReference{
				Name:      srtName,
				Namespace: tKey.Namespace,
			},
		},
	}

	dplName := "t-dpl"
	tDpl := &dplv1.Deployable{
		ObjectMeta: v1.ObjectMeta{
			Name:      dplName,
			Namespace: tKey.Namespace,
		},
		Spec: dplv1.DeployableSpec{
			Template: &runtime.RawExtension{
				Object: &corev1.ConfigMap{
					TypeMeta: v1.TypeMeta{
						Kind: "ConfigMap",
					},
					ObjectMeta: v1.ObjectMeta{
						Name: "cm",
						Annotations: map[string]string{
							dplv1.AnnotationExternalSource: "true",
						},
					},
				},
			},
		},
	}

	defer c.Delete(context.TODO(), refSrt)
	g.Expect(c.Create(context.TODO(), refSrt)).NotTo(gomega.HaveOccurred())

	defer c.Delete(context.TODO(), tCh)
	g.Expect(c.Create(context.TODO(), tCh)).NotTo(gomega.HaveOccurred())

	g.Expect(c.Create(context.TODO(), tDpl)).NotTo(gomega.HaveOccurred())

	chdesc, _ := utils.CreateObjectStorageChannelDescriptor()

	objsync, _ := CreateObjectStoreSynchronizer(mgr.GetConfig(), chdesc, 2)

	objsync.ObjectStore = &utils.FakeObjectStore{}
	g.Expect(objsync.alginClusterResourceWithHost(tCh)).ShouldNot(gomega.HaveOccurred())

	res := &dplv1.Deployable{}
	g.Expect(c.Get(context.TODO(), types.NamespacedName{Name: dplName, Namespace: tKey.Namespace}, res)).Should(gomega.HaveOccurred())
}

//new deployable should create from hostResMap
func Test_alginClusterResourceWithHost_createDplBasedOnHost(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(gomega.HaveOccurred())

	g.Expect(err).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	c := mgr.GetClient()

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	tKey := types.NamespacedName{Name: "tch", Namespace: "tns"}

	srtName := "srt-ref"
	refSrt := &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      srtName,
			Namespace: tKey.Namespace,
		},
		Data: map[string][]byte{
			utils.SecretMapKeyAccessKeyID:     {},
			utils.SecretMapKeySecretAccessKey: {},
		},
	}

	tCh := &chv1.Channel{
		ObjectMeta: v1.ObjectMeta{
			Name:      tKey.Name,
			Namespace: tKey.Namespace,
		},
		Spec: chv1.ChannelSpec{
			Type:     chv1.ChannelType("objectbucket"),
			Pathname: "/" + tKey.Name,
			SecretRef: &corev1.ObjectReference{
				Name:      srtName,
				Namespace: tKey.Namespace,
			},
		},
	}

	dplName := "t-dpl"

	defer c.Delete(context.TODO(), refSrt)
	g.Expect(c.Create(context.TODO(), refSrt)).NotTo(gomega.HaveOccurred())

	defer c.Delete(context.TODO(), tCh)
	g.Expect(c.Create(context.TODO(), tCh)).NotTo(gomega.HaveOccurred())

	chdesc, _ := utils.CreateObjectStorageChannelDescriptor()

	objsync, _ := CreateObjectStoreSynchronizer(mgr.GetConfig(), chdesc, 2)

	objsync.ObjectStore = &utils.FakeObjectStore{
		Clt: map[string]map[string]utils.DeployableObject{
			tKey.Name: {
				dplName: {
					Name:    dplName,
					Content: []byte{},
				},
			},
		},
	}

	g.Expect(objsync.alginClusterResourceWithHost(tCh)).ShouldNot(gomega.HaveOccurred())

	res := &dplv1.Deployable{}
	g.Expect(c.Get(context.TODO(), types.NamespacedName{Name: dplName, Namespace: tKey.Namespace}, res)).ShouldNot(gomega.HaveOccurred())

	c.Delete(context.TODO(), res)
}

//deployable should update from hostResMap
func Test_alginClusterResourceWithHost_updateDplBasedOnHost(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(gomega.HaveOccurred())

	g.Expect(err).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	c := mgr.GetClient()

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	tKey := types.NamespacedName{Name: "tch", Namespace: "tns"}

	srtName := "srt-ref"
	refSrt := &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      srtName,
			Namespace: tKey.Namespace,
		},
		Data: map[string][]byte{
			utils.SecretMapKeyAccessKeyID:     {},
			utils.SecretMapKeySecretAccessKey: {},
		},
	}

	tCh := &chv1.Channel{
		ObjectMeta: v1.ObjectMeta{
			Name:      tKey.Name,
			Namespace: tKey.Namespace,
		},
		Spec: chv1.ChannelSpec{
			Type:     chv1.ChannelType("objectbucket"),
			Pathname: "/" + tKey.Name,
			SecretRef: &corev1.ObjectReference{
				Name:      srtName,
				Namespace: tKey.Namespace,
			},
		},
	}

	dplName := "t-dpl"
	tDpl := &dplv1.Deployable{
		TypeMeta: v1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "apps.open-cluster-management.io/v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      dplName,
			Namespace: tKey.Namespace,
		},
		Spec: dplv1.DeployableSpec{
			Template: &runtime.RawExtension{
				Object: &corev1.ConfigMap{
					TypeMeta: v1.TypeMeta{
						Kind:       "ConfigMap",
						APIVersion: "v1",
					},
					ObjectMeta: v1.ObjectMeta{
						Name: "cm",
						Annotations: map[string]string{
							dplv1.AnnotationExternalSource: "true",
						},
					},
				},
			},
		},
	}

	expectedConfigMapName := "cm-v2"
	hostDpl := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "ConfigMap",
			"apiVersion": "v1",
			"metadata": map[string]interface{}{
				"name":      expectedConfigMapName,
				"namespace": tKey.Namespace,
				"annotations": map[string]string{
					dplv1.AnnotationExternalSource: "true",
					dplv1.AnnotationHosting:        tKey.String(),
				},
			},
			"spec": map[string]interface{}{},
		},
	}

	defer c.Delete(context.TODO(), refSrt)
	g.Expect(c.Create(context.TODO(), refSrt)).NotTo(gomega.HaveOccurred())

	defer c.Delete(context.TODO(), tCh)
	g.Expect(c.Create(context.TODO(), tCh)).NotTo(gomega.HaveOccurred())

	g.Expect(c.Create(context.TODO(), tDpl)).NotTo(gomega.HaveOccurred())

	chdesc, _ := utils.CreateObjectStorageChannelDescriptor()

	objsync, _ := CreateObjectStoreSynchronizer(mgr.GetConfig(), chdesc, 2)

	eTpl, err := hostDpl.MarshalJSON()

	g.Expect(err).Should(gomega.BeNil())

	objsync.ObjectStore = &utils.FakeObjectStore{
		Clt: map[string]map[string]utils.DeployableObject{
			tKey.Name: {
				dplName: {
					Name:    dplName,
					Content: eTpl,
				},
			},
		},
	}

	g.Expect(objsync.alginClusterResourceWithHost(tCh)).ShouldNot(gomega.HaveOccurred())

	res := &dplv1.Deployable{}
	g.Expect(c.Get(context.TODO(), types.NamespacedName{Name: dplName, Namespace: tKey.Namespace}, res)).ShouldNot(gomega.HaveOccurred())

	assertDeployableTemplate(t, res, expectedConfigMapName)
}

func assertDeployableTemplate(t *testing.T, dpl *dplv1.Deployable, eCmName string) {
	tpl := &unstructured.Unstructured{}
	if err := json.Unmarshal(dpl.Spec.Template.Raw, tpl); err != nil {
		t.Errorf("failed to unmarshal template for %v\n", dpl.Name)
	}

	if tpl.GetName() != eCmName {
		t.Errorf("failed to update the deployable %v payload when object host updated, got template %v", dpl.Name, tpl)
	}
}
