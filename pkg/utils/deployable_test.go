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

package utils_test // this way, the test will access the package as a client
import (
	"context"
	"log"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appv1alpha1 "github.com/IBM/multicloud-operators-channel/pkg/apis/app/v1alpha1"
	"github.com/IBM/multicloud-operators-channel/pkg/utils"
	dplv1alpha1 "github.com/IBM/multicloud-operators-deployable/pkg/apis/app/v1alpha1"
	"github.com/google/go-cmp/cmp"
	"github.com/onsi/gomega"
)

type Expected struct {
	pdpl  string // the name of the parent dpl
	cdpls string // this will check the child dpl and it's name
	err   error
}

func TestFindDeployableForChannelsInMap(t *testing.T) {
	testCases := []struct {
		desc   string
		dpl    *dplv1alpha1.Deployable
		ch     map[string]string
		expect Expected
	}{
		{desc: "full dpl family", dpl: TestDpls[dplNode], ch: map[string]string{chName: "test"}, expect: Expected{pdpl: dplParent, cdpls: dplChild, err: nil}},
		{desc: "orphan dpl", dpl: TestDpls[dplOrphan], ch: map[string]string{chName: "test"}, expect: Expected{pdpl: "", cdpls: "", err: nil}},
	}

	listDplObj(c)

	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			p, dpls, err := utils.FindDeployableForChannelsInMap(c, tC.dpl, tC.ch)
			if assertDpls(tC.expect, chName, p, dpls[chName], err) {
				t.Errorf("wanted %#v, got %v %v %v", tC.expect, dpls, p, err)
			}
		})
	}
}

func listDplObj(cl client.Client) {
	dpllist := &dplv1alpha1.DeployableList{}
	err := cl.List(context.TODO(), dpllist, &client.ListOptions{})

	if err != nil {
		log.Printf("Failed to list deployables for")
		return
	}

	for _, dpl := range dpllist.Items {
		log.Printf("have dpl %v/%v", dpl.GetName(), dpl.GetNamespace())
	}

	log.Printf("In total %v dpl is found", len(dpllist.Items))
}

func assertDpls(expect Expected, cname string, pdpls *dplv1alpha1.Deployable, dpls *dplv1alpha1.Deployable, err error) bool {
	klog.Infof("expected %v, cname %v", expect, cname)

	if expect.err != err {
		return false
	}

	if pdpls == nil || expect.pdpl != pdpls.GetName() {
		return false
	}

	if dpls == nil || dpls.GetName() != expect.cdpls {
		return false
	}

	return true
}

func TestValidateDeployableInChannel(t *testing.T) {
	testNamespace := "a-ns"
	testDplName := "tDpl"
	testCh := "tCh"
	theAnno := map[string]string{"who": "bear"}
	theAnnoA := map[string]string{"who": "bear", "what": "beer"}
	theAnnoB := map[string]string{"hmm": "eh"}

	testCases := []struct {
		desc string
		dpl  *dplv1alpha1.Deployable
		ch   *appv1alpha1.Channel
		want bool
	}{
		{
			desc: "empty channel",
			dpl: &dplv1alpha1.Deployable{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testDplName,
					Namespace:   testNamespace,
					Annotations: theAnno,
				},
				Spec: dplv1alpha1.DeployableSpec{
					Template: &runtime.RawExtension{},
				},
			},
			want: false,
		},
		{
			desc: "without gate channel and deployable due to same namespace",
			dpl: &dplv1alpha1.Deployable{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testDplName,
					Namespace:   testNamespace,
					Annotations: theAnnoA,
				},
				Spec: dplv1alpha1.DeployableSpec{
					Template: &runtime.RawExtension{},
				},
			},
			ch: &appv1alpha1.Channel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testCh,
					Namespace: testNamespace,
				},
			},
			want: true,
		},
		{
			desc: "without gate channel and deployable doesn't match",
			dpl: &dplv1alpha1.Deployable{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testDplName,
					Namespace:   testNamespace,
					Annotations: theAnnoB,
				},
				Spec: dplv1alpha1.DeployableSpec{
					Template: &runtime.RawExtension{},
				},
			},
			ch: &appv1alpha1.Channel{
				ObjectMeta: metav1.ObjectMeta{
					Name: testCh,
				},
			},
			want: false,
		},
		{
			desc: "with gate channel and deployable doesn't match anno",
			dpl: &dplv1alpha1.Deployable{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testDplName,
					Namespace:   testNamespace,
					Annotations: theAnnoB,
				},
				Spec: dplv1alpha1.DeployableSpec{
					Template: &runtime.RawExtension{},
				},
			},
			ch: &appv1alpha1.Channel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testCh,
					Namespace: testNamespace,
				},
				Spec: appv1alpha1.ChannelSpec{
					Gates: &appv1alpha1.ChannelGate{
						Annotations: theAnno,
					},
				},
			},
			want: false,
		},
		{
			desc: "with gate channel and deployable doesn't match anno",
			dpl: &dplv1alpha1.Deployable{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testDplName,
					Namespace: testNamespace,
				},
				Spec: dplv1alpha1.DeployableSpec{
					Template: &runtime.RawExtension{},
				},
			},
			ch: &appv1alpha1.Channel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testCh,
					Namespace: testNamespace,
				},
				Spec: appv1alpha1.ChannelSpec{
					Gates: &appv1alpha1.ChannelGate{
						Annotations: theAnno,
					},
				},
			},
			want: false,
		},
		{
			desc: "with gate channel and deployable match",
			dpl: &dplv1alpha1.Deployable{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testDplName,
					Namespace:   testNamespace,
					Annotations: theAnnoA,
				},
				Spec: dplv1alpha1.DeployableSpec{
					Template: &runtime.RawExtension{},
				},
			},
			ch: &appv1alpha1.Channel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testCh,
					Namespace: testNamespace,
				},
				Spec: appv1alpha1.ChannelSpec{
					Gates: &appv1alpha1.ChannelGate{
						Annotations: theAnno,
					},
				},
			},
			want: true,
		},
	}

	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			got := utils.ValidateDeployableInChannel(tC.dpl, tC.ch)
			if got != tC.want {
				t.Errorf("wanted %v, got %v", tC.want, got)
			}
		})
	}
}

func TestCleanupDeployables(t *testing.T) {
	testNamespace := "a-ns"
	testDplName := "t-dpl"
	theAnnoA := map[string]string{"who": "bear", "what": "beer"}

	dplObj := &dplv1alpha1.Deployable{
		ObjectMeta: metav1.ObjectMeta{
			Name:        testDplName,
			Namespace:   testNamespace,
			Annotations: theAnnoA},
		Spec: dplv1alpha1.DeployableSpec{
			Template: &runtime.RawExtension{
				Object: &v1.ConfigMap{},
			},
		},
	}

	dplObj1 := &dplv1alpha1.Deployable{
		ObjectMeta: metav1.ObjectMeta{
			Name:        testDplName + "1",
			Namespace:   testNamespace,
			Annotations: theAnnoA},
		Spec: dplv1alpha1.DeployableSpec{
			Template: &runtime.RawExtension{
				Object: &v1.ConfigMap{},
			},
		},
	}

	testCases := []struct {
		desc   string
		hubClt client.Client
		tCh    types.NamespacedName
		want   error
	}{
		{
			desc:   "",
			hubClt: c,
			tCh:    types.NamespacedName{Name: "a", Namespace: "none-exist"},
			want:   nil,
		},
		{
			desc:   "",
			hubClt: c,
			want:   nil,
		},
	}

	dplList := []*dplv1alpha1.Deployable{dplObj, dplObj1}
	delFuncs := []func(){}

	g := gomega.NewGomegaWithT(t)
	for _, d := range dplList {
		delFunc, err := GenerateCRsAtLocalKube(c, d)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		delFuncs = append(delFuncs, delFunc)
	}

	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			got := utils.CleanupDeployables(tC.hubClt, tC.tCh)
			if diff := cmp.Diff(got, tC.want); diff != "" {
				t.Errorf("CleanupDeployables mismatch (-want, +got):\n%s", diff)
			}
		})
	}

	for _, fn := range delFuncs {
		fn()
	}
}
