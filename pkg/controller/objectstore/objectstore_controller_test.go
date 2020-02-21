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

package objectstore

import (
	"context"
	"testing"
	"time"

	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	chnv1alpha1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/app/v1alpha1"
	"github.com/open-cluster-management/multicloud-operators-channel/pkg/utils"
	dplv1alpha1 "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis/app/v1alpha1"
)

const timeout = time.Second * 2

func TestObjstoreController(t *testing.T) {
	testNamespace := "a-ns"
	testDplName := "tdpl"
	theAnno := map[string]string{"who": "bear"}

	expectedResult := reconcile.Result{}

	g := gomega.NewGomegaWithT(t)

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(gomega.HaveOccurred())

	chDesc, err := utils.CreateObjectStorageChannelDescriptor()
	g.Expect(err).NotTo(gomega.HaveOccurred())

	rec := newReconciler(mgr, chDesc)
	recFn, result := SetupTestReconcile(rec)

	g.Expect(add(mgr, recFn)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	c := mgr.GetClient()

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	dpl := &dplv1alpha1.Deployable{
		ObjectMeta: metav1.ObjectMeta{
			Name:        testDplName,
			Namespace:   testNamespace,
			Annotations: theAnno,
		},
		Spec: dplv1alpha1.DeployableSpec{
			Template: &runtime.RawExtension{
				Object: &v1.ConfigMap{},
			},
		},
	}

	ctx := context.TODO()
	g.Expect(c.Create(ctx, dpl)).NotTo(gomega.HaveOccurred())

	defer c.Delete(ctx, dpl)

	g.Eventually(result, timeout).Should(gomega.Receive(gomega.Equal(expectedResult)))
}

func Test_getChannelForNamespace(t *testing.T) {
	testNamespace := "a-ns"
	testCh := "obj"

	g := gomega.NewGomegaWithT(t)
	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(gomega.HaveOccurred())

	chDesc, err := utils.CreateObjectStorageChannelDescriptor()
	g.Expect(err).NotTo(gomega.HaveOccurred())

	rec := newReconciler(mgr, chDesc)
	recFn, _ := SetupTestReconcile(rec)

	g.Expect(add(mgr, recFn)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	c := mgr.GetClient()

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	testCases := []struct {
		desc   string
		tRec   *ReconcileDeployable
		dfChan *chnv1alpha1.Channel
		tName  string
		wantCh *chnv1alpha1.Channel
	}{
		{
			desc: "none channel at hub",
			tRec: &ReconcileDeployable{
				KubeClient:        c,
				ChannelDescriptor: &utils.ChannelDescriptor{},
			},
			tName:  testNamespace,
			wantCh: nil,
		},
		{
			desc: "channel without spec type at hub",
			tRec: &ReconcileDeployable{
				KubeClient:        c,
				ChannelDescriptor: &utils.ChannelDescriptor{},
			},
			dfChan: &chnv1alpha1.Channel{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCh,
					Namespace:   testNamespace,
					Annotations: map[string]string{},
				},
			},
			tName:  testNamespace,
			wantCh: nil,
		},
		{
			desc: "channel with spec type at hub",
			tRec: &ReconcileDeployable{
				KubeClient:        c,
				ChannelDescriptor: &utils.ChannelDescriptor{},
			},
			dfChan: &chnv1alpha1.Channel{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCh,
					Namespace:   testNamespace,
					Annotations: map[string]string{},
				},
				Spec: chnv1alpha1.ChannelSpec{
					Type: chnv1alpha1.ChannelTypeObjectBucket,
				},
			},
			tName: testNamespace,
			wantCh: &chnv1alpha1.Channel{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCh,
					Namespace:   testNamespace,
					Annotations: map[string]string{},
				},
				Spec: chnv1alpha1.ChannelSpec{
					Type: chnv1alpha1.ChannelTypeObjectBucket,
				},
			},
		},
		{
			desc: "channel with spec type at hub",
			tRec: &ReconcileDeployable{
				KubeClient:        c,
				ChannelDescriptor: &utils.ChannelDescriptor{},
			},
			dfChan: &chnv1alpha1.Channel{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCh,
					Namespace:   testNamespace,
					Annotations: map[string]string{},
				},
				Spec: chnv1alpha1.ChannelSpec{
					Type: chnv1alpha1.ChannelTypeGitHub,
				},
			},
			tName:  testNamespace,
			wantCh: nil,
		},
	}
	ctx := context.TODO()

	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			tC.tRec.KubeClient.Create(ctx, tC.dfChan)
			defer tC.tRec.KubeClient.Delete(ctx, tC.dfChan)
			time.Sleep(1 * time.Second)
			gotCh, _ := tC.tRec.getChannelForNamespace(testNamespace)

			assertChannelObject(t, tC.wantCh, gotCh)
		})
	}
}

func assertChannelObject(t *testing.T, want, got *chnv1alpha1.Channel) {
	t.Helper()

	if want == nil && got == nil {
		return
	}

	if want == nil && got != nil {
		t.Errorf("wanted %v got %v", want, got)
		return
	}

	if want != nil && got == nil {
		t.Errorf("wanted %v got %v", want, got)
		return
	}

	if want.GetName() == got.GetName() && want.GetNamespace() == got.GetNamespace() {
		return
	}
}
