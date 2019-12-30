// Licensed Materials - Property of IBM
// (c) Copyright IBM Corporation 2016, 2019. All Rights Reserved.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP  Schedule Contract with IBM Corp.

package utils_test // this way, the test will access the package as a client
import (
	"context"
	"log"
	"testing"

	"github.com/IBM/multicloud-operators-channel/pkg/utils"
	dplv1alpha1 "github.com/IBM/multicloud-operators-deployable/pkg/apis/app/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// parentDpl

// curDpl
var parentName = "generateddpl"

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
	err := cl.List(context.TODO(), &client.ListOptions{}, dpllist)
	if err != nil {
		log.Printf("Failed to list deployables for")
		return
	}

	for _, dpl := range dpllist.Items {
		log.Printf("have dpl %v/%v", dpl.GetName(), dpl.GetNamespace())
	}
	log.Printf("In total %v dpl is found", len(dpllist.Items))
	return
}

func assertDpls(expect Expected, cname string, pdpls *dplv1alpha1.Deployable, dpls *dplv1alpha1.Deployable, err error) bool {
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
