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

	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/IBM/multicloud-operators-channel/pkg/utils"
	dplv1alpha1 "github.com/IBM/multicloud-operators-deployable/pkg/apis/app/v1alpha1"
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
