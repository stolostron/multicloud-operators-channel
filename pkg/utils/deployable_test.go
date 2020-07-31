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
	"encoding/json"
	"log"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	mgr "sigs.k8s.io/controller-runtime/pkg/manager"

	tlog "github.com/go-logr/logr/testing"
	"github.com/google/go-cmp/cmp"
	"github.com/onsi/gomega"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"
	"github.com/open-cluster-management/multicloud-operators-channel/pkg/utils"
	dplv1 "github.com/open-cluster-management/multicloud-operators-deployable/pkg/apis/apps/v1"
)

const (
	dplNode                       = "dplnode"
	dplParent                     = "dplparent"
	dplChild                      = "dplchild"
	dplOrphan                     = "dplorphan"
	CtrlDeployableIndexer         = "origin-deployable"
	CtrlGenerateDeployableIndexer = "generated-deployable"
)

type dplElements struct {
	name string
	ns   string
	ch   string
}

var configmap, _ = json.Marshal(v1.ConfigMap{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "defaultcfg",
		Namespace: "default",
	},
})

func dplGenerator(dpl dplElements) *dplv1.Deployable {
	return &dplv1.Deployable{
		TypeMeta: metav1.TypeMeta{
			Kind:       utils.DeployableTypeKind,
			APIVersion: utils.DeployableTypeAPIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      dpl.name,
			Namespace: dpl.ns,
		},
		Spec: dplv1.DeployableSpec{
			Template: &runtime.RawExtension{Raw: configmap},
			Channels: []string{dpl.ch},
		},
	}
}

// create some pre-define dpl obj to test the relationship
func initObjQueue() ([]*dplv1.Deployable, map[string]*dplv1.Deployable) {
	dplNs := "default"
	chName := "qa"

	var initObjs []*dplv1.Deployable

	TestDpls := make(map[string]*dplv1.Deployable)

	dpls := []dplElements{
		{name: dplNode, ns: dplNs, ch: chName},
		{name: dplParent, ns: dplNs, ch: chName},
		{name: dplChild, ns: dplNs, ch: chName},
		{name: dplOrphan, ns: dplNs, ch: chName},
	}

	for _, dpl := range dpls {
		t := dplGenerator(dpl)

		if dpl.name == dplNode || dpl.name == dplOrphan {
			TestDpls[dpl.name] = t
		}

		initObjs = append(initObjs, dplGenerator(dpl))
	}

	return initObjs, TestDpls
}

func deploybObjects(ctx context.Context, c client.Client, objs []*dplv1.Deployable) {
	for _, obj := range objs {
		c.Create(ctx, obj)
	}
}

func deleteObjects(ctx context.Context, c client.Client, objs []*dplv1.Deployable) {
	for _, obj := range objs {
		c.Delete(ctx, obj)
	}
}

type Expected struct {
	pdpl  string // the name of the parent dpl
	cdpls string // this will check the child dpl and it's name
	err   error
}

func TestFindDeployableForChannelsInMap(t *testing.T) {
	var chName = "qa"

	initObjs, TestDpls := initObjQueue()

	ctx := context.TODO()
	deploybObjects(ctx, c, initObjs)

	defer deleteObjects(ctx, c, initObjs)

	testCases := []struct {
		desc   string
		dpl    *dplv1.Deployable
		ch     map[string]string
		expect Expected
	}{
		{desc: "full dpl family", dpl: TestDpls[dplNode], ch: map[string]string{chName: "test"}, expect: Expected{pdpl: dplParent, cdpls: dplChild, err: nil}},
		{desc: "orphan dpl", dpl: TestDpls[dplOrphan], ch: map[string]string{chName: "test"}, expect: Expected{pdpl: "", cdpls: "", err: nil}},
	}

	listDplObj(c)

	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			p, dpls, err := utils.RebuildDeployableRelationshipGraph(c, tC.dpl, tC.ch, tlog.NullLogger{})
			if assertDpls(tC.expect, p, dpls[chName], err) {
				t.Errorf("wanted %#v, got %v %v %v", tC.expect, dpls, p, err)
			}
		})
	}
}

func listDplObj(cl client.Client) {
	dpllist := &dplv1.DeployableList{}
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

func assertDpls(expect Expected, pdpls *dplv1.Deployable, dpls *dplv1.Deployable, err error) bool {
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
		dpl  *dplv1.Deployable
		ch   *chv1.Channel
		want bool
	}{
		{
			desc: "empty channel",
			dpl: &dplv1.Deployable{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testDplName,
					Namespace:   testNamespace,
					Annotations: theAnno,
				},
				Spec: dplv1.DeployableSpec{
					Template: &runtime.RawExtension{},
				},
			},
			want: false,
		},
		{
			desc: "without gate channel and deployable due to same namespace",
			dpl: &dplv1.Deployable{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testDplName,
					Namespace:   testNamespace,
					Annotations: theAnnoA,
				},
				Spec: dplv1.DeployableSpec{
					Template: &runtime.RawExtension{},
				},
			},
			ch: &chv1.Channel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testCh,
					Namespace: testNamespace,
				},
			},
			want: true,
		},
		{
			desc: "without gate channel and deployable doesn't match",
			dpl: &dplv1.Deployable{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testDplName,
					Namespace:   testNamespace,
					Annotations: theAnnoB,
				},
				Spec: dplv1.DeployableSpec{
					Template: &runtime.RawExtension{},
				},
			},
			ch: &chv1.Channel{
				ObjectMeta: metav1.ObjectMeta{
					Name: testCh,
				},
			},
			want: false,
		},
		{
			desc: "with gate channel and deployable doesn't match anno",
			dpl: &dplv1.Deployable{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testDplName,
					Namespace:   testNamespace,
					Annotations: theAnnoB,
				},
				Spec: dplv1.DeployableSpec{
					Template: &runtime.RawExtension{},
				},
			},
			ch: &chv1.Channel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testCh,
					Namespace: testNamespace,
				},
				Spec: chv1.ChannelSpec{
					Gates: &chv1.ChannelGate{
						Annotations: theAnno,
					},
				},
			},
			want: false,
		},
		{
			desc: "with gate channel and deployable doesn't match anno",
			dpl: &dplv1.Deployable{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testDplName,
					Namespace: testNamespace,
				},
				Spec: dplv1.DeployableSpec{
					Template: &runtime.RawExtension{},
				},
			},
			ch: &chv1.Channel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testCh,
					Namespace: testNamespace,
				},
				Spec: chv1.ChannelSpec{
					Gates: &chv1.ChannelGate{
						Annotations: theAnno,
					},
				},
			},
			want: false,
		},
		{
			desc: "with gate channel and deployable match",
			dpl: &dplv1.Deployable{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testDplName,
					Namespace:   testNamespace,
					Annotations: theAnnoA,
				},
				Spec: dplv1.DeployableSpec{
					Template: &runtime.RawExtension{},
				},
			},
			ch: &chv1.Channel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testCh,
					Namespace: testNamespace,
				},
				Spec: chv1.ChannelSpec{
					Gates: &chv1.ChannelGate{
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

func TestValidateDeployableToChannel(t *testing.T) {
	testNamespace := "a-ns"
	testDplName := "tDpl"
	testCh := "tCh"
	theAnno := map[string]string{"who": "bear"}
	theAnnoA := map[string]string{"who": "bear", "what": "beer"}

	testCases := []struct {
		desc string
		dpl  *dplv1.Deployable
		ch   *chv1.Channel
		want bool
	}{
		{
			desc: "deployable can be deployed to channel",
			dpl: &dplv1.Deployable{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testDplName,
					Namespace:   testNamespace,
					Annotations: theAnnoA,
				},
				Spec: dplv1.DeployableSpec{
					Template: &runtime.RawExtension{},
				},
			},
			ch: &chv1.Channel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testCh,
					Namespace: testNamespace,
				},
			},
			want: false,
		},
		{
			desc: "cant due to miss channel sourcenamespace",
			dpl: &dplv1.Deployable{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testDplName,
					Namespace:   testNamespace,
					Annotations: theAnnoA,
				},
				Spec: dplv1.DeployableSpec{
					Template: &runtime.RawExtension{},
				},
			},
			ch: &chv1.Channel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testCh,
					Namespace: testNamespace,
				},
				Spec: chv1.ChannelSpec{
					Gates: &chv1.ChannelGate{
						Annotations: theAnno,
					},
				},
			},
			want: false,
		},
		{
			desc: "can deploy channel dont have gate",
			dpl: &dplv1.Deployable{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testDplName,
					Namespace:   testNamespace,
					Annotations: theAnnoA,
				},
				Spec: dplv1.DeployableSpec{
					Template: &runtime.RawExtension{},
					Channels: []string{testCh},
				},
			},
			ch: &chv1.Channel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testCh,
					Namespace: testNamespace,
				},
			},
			want: true,
		},
		{
			desc: "can deploy due to channel gate is equal to deployable",
			dpl: &dplv1.Deployable{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testDplName,
					Namespace:   testNamespace,
					Annotations: theAnnoA,
				},
				Spec: dplv1.DeployableSpec{
					Template: &runtime.RawExtension{},
					Channels: []string{testCh},
				},
			},
			ch: &chv1.Channel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testCh,
					Namespace: testNamespace,
				},
				Spec: chv1.ChannelSpec{
					Gates: &chv1.ChannelGate{
						Annotations: theAnno,
					},
				},
			},
			want: true,
		},
		{
			desc: "can deploy due to channel point to the sourcenamespace of  deployable and gate is disabled",
			dpl: &dplv1.Deployable{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testDplName,
					Namespace:   testNamespace,
					Annotations: theAnnoA,
				},
				Spec: dplv1.DeployableSpec{
					Template: &runtime.RawExtension{},
					Channels: []string{testCh},
				},
			},
			ch: &chv1.Channel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testCh,
					Namespace: testNamespace,
				},
				Spec: chv1.ChannelSpec{
					Gates: &chv1.ChannelGate{
						Annotations: theAnno,
					},
					SourceNamespaces: []string{testNamespace},
				},
			},
			want: true,
		},
	}

	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			got := utils.ValidateDeployableToChannel(tC.dpl, tC.ch)
			if got != tC.want {
				t.Errorf("ValidateDeployableToChannel failed wanted %v got %v", tC.want, got)
			}
		})
	}
}

func TestCleanupDeployables(t *testing.T) {
	testNamespace := "a-ns"
	testDplName := "t-dpl"
	theAnnoA := map[string]string{"who": "bear", "what": "beer"}

	dplObj := &dplv1.Deployable{
		ObjectMeta: metav1.ObjectMeta{
			Name:        testDplName,
			Namespace:   testNamespace,
			Annotations: theAnnoA},
		Spec: dplv1.DeployableSpec{
			Template: &runtime.RawExtension{
				Object: &v1.ConfigMap{},
			},
		},
	}

	dplObj1 := &dplv1.Deployable{
		ObjectMeta: metav1.ObjectMeta{
			Name:        testDplName + "1",
			Namespace:   testNamespace,
			Annotations: theAnnoA},
		Spec: dplv1.DeployableSpec{
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

	dplList := []*dplv1.Deployable{dplObj, dplObj1}
	delFuncs := []func(){}

	g := gomega.NewGomegaWithT(t)

	for _, d := range dplList {
		delFunc, err := GenerateCRsAtLocalKube(c, d)
		g.Expect(err).NotTo(gomega.HaveOccurred())

		delFuncs = append(delFuncs, delFunc)
	}

	stop := make(chan struct{})
	defer close(stop)

	k8sManager, err := mgr.New(cfg, mgr.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(gomega.HaveOccurred())

	g.Expect(addIndexFiled(k8sManager)).Should(gomega.Succeed())

	go func() {
		g.Expect(k8sManager.Start(stop)).ToNot(gomega.HaveOccurred())
	}()

	k8sClient := k8sManager.GetClient()
	g.Expect(k8sClient).ToNot(gomega.BeNil())

	g.Expect(k8sManager.GetCache().WaitForCacheSync(stop)).Should(gomega.BeTrue())

	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			got := utils.CleanupDeployables(k8sClient, tC.tCh)
			if diff := cmp.Diff(got, tC.want); diff != "" {
				t.Errorf("CleanupDeployables mismatch (-want, +got):\n%s", diff)
			}
		})
	}

	for _, fn := range delFuncs {
		fn()
	}
}

func addIndexFiled(k8sManager mgr.Manager) error {
	if err := k8sManager.GetFieldIndexer().IndexField(context.TODO(), &dplv1.Deployable{}, CtrlDeployableIndexer, func(rawObj runtime.Object) []string {
		// grab the job object, extract the owner...
		dpl := rawObj.(*dplv1.Deployable)
		anno := dpl.GetAnnotations()
		if len(anno) == 0 {
			return nil
		}
		// this make sure the indexer will only be applied on non-generated
		// deployable
		if _, ok := anno[chv1.KeyChannelSource]; ok {
			return nil
		}

		// ...and if so, return it
		return []string{"true"}
	}); err != nil {
		return err
	}

	if err := k8sManager.GetFieldIndexer().IndexField(context.TODO(), &dplv1.Deployable{}, CtrlGenerateDeployableIndexer, func(rawObj runtime.Object) []string {
		// grab the job object, extract the owner...
		dpl := rawObj.(*dplv1.Deployable)
		anno := dpl.GetAnnotations()
		if len(anno) == 0 {
			return nil
		}
		// this make sure the indexer will only be applied on generated
		// deployable
		if _, ok := anno[chv1.KeyChannelSource]; !ok {
			return nil
		}

		// ...and if so, return it
		return []string{"true"}
	}); err != nil {
		return err
	}

	return nil
}

func TestGenerateDeployableForChannel(t *testing.T) {
	testNamespace := "a-ns"
	testDplName := "tDpl"
	testCh := "tCh"
	theAnnoA := map[string]string{"who": "bear", "what": "beer"}
	labels := map[string]string{
		"you": "yes",
		"who": "no",
	}

	chkey := types.NamespacedName{Name: testCh, Namespace: testNamespace}

	targetAnno := theAnnoA
	targetAnno[dplv1.AnnotationLocal] = "false"
	targetAnno[chv1.KeyChannel] = chkey.String()
	targetAnno[chv1.KeyChannelSource] = types.NamespacedName{Name: testDplName, Namespace: testNamespace}.String()
	targetAnno[dplv1.AnnotationIsGenerated] = "true"

	testCases := []struct {
		desc  string
		dpl   *dplv1.Deployable
		chKey types.NamespacedName
		want  *dplv1.Deployable
	}{
		{
			desc: "copy over all the fields and adding annotations from channel",
			dpl: &dplv1.Deployable{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testDplName,
					Namespace:   testNamespace,
					Labels:      labels,
					Annotations: theAnnoA},
				Spec: dplv1.DeployableSpec{
					Template: &runtime.RawExtension{
						Object: &v1.ConfigMap{},
					},
				},
			},
			chKey: chkey,
			want: &dplv1.Deployable{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName:    testDplName + "-",
					Labels:          labels,
					Namespace:       testNamespace,
					Annotations:     targetAnno,
					OwnerReferences: []metav1.OwnerReference{{Name: testDplName}},
				},
				Spec: dplv1.DeployableSpec{
					Template: &runtime.RawExtension{
						Object: &v1.ConfigMap{},
					},
				},
			},
		},
		{
			desc:  "nil deployable",
			dpl:   nil,
			chKey: chkey,
			want:  nil,
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			got, err := utils.GenerateDeployableForChannel(tC.dpl, tC.chKey)
			if err != nil {
				t.Errorf("wanted %#v got %#v", tC.want, got)
			}

			if diff := cmp.Diff(tC.want, got); diff != "" {
				t.Errorf("GenerateDeployableForChannel fail (-wantl +got):\n%s", diff)
			}
		})
	}
}

func TestSplitStringToKey(t *testing.T) {
	var tests = []struct {
		name     string
		given    string
		expected types.NamespacedName
	}{
		{name: "only have name", given: "/test", expected: types.NamespacedName{Name: "test", Namespace: "default"}},
		{name: "have name and ns", given: "chn/test", expected: types.NamespacedName{Name: "test", Namespace: "chn"}},
		{name: "only have ns", given: "chn/", expected: types.NamespacedName{}},
		{name: "only have name", given: "", expected: types.NamespacedName{}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			actual := utils.SplitStringToTypes(tt.given)
			if actual != tt.expected {
				t.Errorf("(%s): expected %s, actual %s", tt.given, tt.expected, actual)
			}
		})
	}
}
