// Licensed Materials - Property of IBM
// (c) Copyright IBM Corporation 2016, 2019. All Rights Reserved.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP  Schedule Contract with IBM Corp.

package utils_test

import (
	"encoding/json"

	dplv1alpha1 "github.com/IBM/multicloud-operators-deployable/pkg/apis/app/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type dplElements struct {
	name string
	ns   string
	ch   string
}

var configmap, _ = json.Marshal(corev1.ConfigMap{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "defaultcfg",
		Namespace: "default",
	},
})

func DplGenerator(dpl dplElements) *dplv1alpha1.Deployable {
	return &dplv1alpha1.Deployable{
		TypeMeta: metav1.TypeMeta{
			Kind:       "app.ibm.com",
			APIVersion: "v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      dpl.name,
			Namespace: dpl.ns,
		},
		Spec: dplv1alpha1.DeployableSpec{
			Template: &runtime.RawExtension{Raw: configmap},
			Channels: []string{dpl.ch},
		},
	}
}

var dplNode = "dplnode"
var dplParent = "dplparent"
var dplChild = "dplchild"
var dplOrphan = "dplorphan"

var TestDpls = make(map[string]*dplv1alpha1.Deployable)

// create some pre-define dpl obj to test the relationship
func InitObjQueue() {
	dpls := []dplElements{
		{name: dplNode, ns: dplNs, ch: chName},
		{name: dplParent, ns: dplNs, ch: chName},
		{name: dplChild, ns: dplNs, ch: chName},
		{name: dplOrphan, ns: dplNs, ch: chName},
	}

	for _, dpl := range dpls {
		t := DplGenerator(dpl)
		if dpl.name == dplNode || dpl.name == dplOrphan {
			TestDpls[dpl.name] = t
		}
		ObjTobeCreated = append(ObjTobeCreated, DplGenerator(dpl))
	}
}
