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
	"context"
	"encoding/json"
	"testing"

	"github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"
	"github.com/open-cluster-management/multicloud-operators-channel/pkg/utils"
	dplv1 "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis/apps/v1"
)

// 1. clone from git and create deployables for all the resources at git
// 2. delete/update local deployables resource if git resource changed

const (
	helmTests            = "../../../tests/helm/testhelm"
	helmTestsUpdate      = "../../../tests/helm/testhelm-update"
	helmChartsNum        = 2
	helmChartsUpdatedNum = 1
	testSyncInterval     = 2
)

func Test_HelmCloneAndCreateDeployables(t *testing.T) {
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

	sync, err := CreateHelmrepoSynchronizer(mgr.GetConfig(), mgr.GetScheme(), testSyncInterval)

	if err != nil {
		t.Error("failed to create synchronizer")
	}

	chKey := types.NamespacedName{Name: "t-ch", Namespace: "t-ch-ns"}
	tChn := &chv1.Channel{
		ObjectMeta: v1.ObjectMeta{
			Namespace: chKey.Namespace,
			Name:      chKey.Name,
		},
		Spec: chv1.ChannelSpec{
			Type:     chv1.ChannelType("helmrepo"),
			Pathname: helmTests,
		},
	}

	defer c.Delete(context.TODO(), tChn)
	g.Expect(c.Create(context.TODO(), tChn)).NotTo(gomega.HaveOccurred())

	fCh := &chv1.Channel{}
	g.Expect(c.Get(context.TODO(), chKey, fCh))

	sync.syncChannel(fCh, utils.LoadLocalIdx)

	dplList := &dplv1.DeployableList{}

	g.Expect(c.List(context.TODO(), dplList)).NotTo(gomega.HaveOccurred())

	dplCnt := map[string]int{
		"HelmRelease": helmChartsNum,
	}
	assertDeployableList(t, dplList, dplCnt)
}

func assertDeployableList(t *testing.T, dplList *dplv1.DeployableList, dplCnt map[string]int) {
	for _, dpl := range dplList.Items {
		tpl := &unstructured.Unstructured{}
		if err := json.Unmarshal(dpl.Spec.Template.Raw, tpl); err != nil {
			t.Errorf("assertDeployableList failed err: %v", err)
		}

		if _, ok := dplCnt[tpl.GetKind()]; ok {
			dplCnt[tpl.GetKind()]--
		}
	}

	for k, v := range dplCnt {
		if v != 0 {
			t.Errorf("dployable of %v doesn't match the pre-set condition, wanted 0, got %v", k, v)
		}
	}
}

func Test_HelmDeleteOrUpdateDeployables(t *testing.T) {
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

	sync, err := CreateHelmrepoSynchronizer(mgr.GetConfig(), mgr.GetScheme(), testSyncInterval)

	if err != nil {
		t.Error("failed to create synchronizer")
	}

	chKey := types.NamespacedName{Name: "t-ch", Namespace: "t-ch-ns"}
	tChn := &chv1.Channel{
		ObjectMeta: v1.ObjectMeta{
			Namespace: chKey.Namespace,
			Name:      chKey.Name,
		},
		Spec: chv1.ChannelSpec{
			Type:     chv1.ChannelType("helmrepo"),
			Pathname: helmTests,
		},
	}

	defer c.Delete(context.TODO(), tChn)
	g.Expect(c.Create(context.TODO(), tChn)).NotTo(gomega.HaveOccurred())

	fCh := &chv1.Channel{}
	g.Expect(c.Get(context.TODO(), chKey, fCh))

	sync.syncChannel(fCh, utils.LoadLocalIdx)

	dplList := &dplv1.DeployableList{}

	g.Expect(c.List(context.TODO(), dplList)).NotTo(gomega.HaveOccurred())

	dplCnt := map[string]int{
		"HelmRelease": helmChartsNum,
	}
	assertDeployableList(t, dplList, dplCnt)

	fCh.Spec.Pathname = helmTestsUpdate

	sync.syncChannel(fCh, utils.LoadLocalIdx)

	dplListUpdate := &dplv1.DeployableList{}
	g.Expect(c.List(context.TODO(), dplListUpdate)).NotTo(gomega.HaveOccurred())

	dplUpdateCnt := map[string]int{
		"HelmRelease": helmChartsUpdatedNum,
	}
	assertDeployableList(t, dplListUpdate, dplUpdateCnt)
}
