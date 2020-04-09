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
	"strings"
	"testing"

	tlog "github.com/go-logr/logr/testing"
	"github.com/google/go-cmp/cmp"
	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"
	"github.com/open-cluster-management/multicloud-operators-channel/pkg/utils"
)

// this is mainly testing if a Channel resource can be created or not
func TestGenerateChannelMap(t *testing.T) {
	chName := "qa"
	chNs := "ch-qa"

	key := types.NamespacedName{
		Name:      chName,
		Namespace: chNs,
	}
	chObj := &chv1.Channel{
		TypeMeta: metav1.TypeMeta{
			Kind:       utils.ChannelTypeKind,
			APIVersion: utils.ChannelTypeAPIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      chName,
			Namespace: chNs,
		},
		Spec: chv1.ChannelSpec{
			Type: chv1.ChannelType("namespace"),
		},
	}

	g := gomega.NewWithT(t)
	ctx := context.TODO()

	g.Expect(c.Create(ctx, chObj)).NotTo(gomega.HaveOccurred())
	defer c.Delete(ctx, chObj)

	fetched := &chv1.Channel{}
	g.Expect(c.Get(ctx, key, fetched)).NotTo(gomega.HaveOccurred())

	got, _ := utils.GenerateChannelMap(c, tlog.NullLogger{})
	want := map[string]*chv1.Channel{chName: fetched}

	// test cases
	g.Expect(got[chName].Spec).To(gomega.Equal(want[chName].Spec))
	g.Expect(got[chName].ObjectMeta).To(gomega.Equal(want[chName].ObjectMeta))
}

func TestLocateChannel(t *testing.T) {
	chName := "qa"
	chNs := "ch-qa"

	chObj := &chv1.Channel{
		TypeMeta: metav1.TypeMeta{
			Kind:       utils.ChannelTypeKind,
			APIVersion: utils.ChannelTypeAPIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      chName,
			Namespace: chNs,
		},
		Spec: chv1.ChannelSpec{
			Type: chv1.ChannelType("namespace"),
		},
	}

	g := gomega.NewWithT(t)
	ctx := context.TODO()

	g.Expect(c.Create(ctx, chObj)).NotTo(gomega.HaveOccurred())
	defer c.Delete(ctx, chObj)

	got, err := utils.LocateChannel(c, chName)

	if err != nil {
		t.Fatalf("fatal error at the local channel test, %v\n", err)
	}

	g.Expect(got).NotTo(gomega.BeNil(), "channel is nil")

	g.Expect((*got).GetName()).Should(gomega.Equal(chName), "There's a match")

	got, _ = utils.LocateChannel(c, "")

	g.Expect(got).Should(gomega.BeNil(), "There's no match 1")

	got, _ = utils.LocateChannel(c, "wrongName")

	g.Expect(got).Should(gomega.BeNil(), "There's no match 2")
}

func TestUpdateServingChannel(t *testing.T) {
	chName := "qa"
	chNs := "ch-qa"

	key := types.NamespacedName{
		Name:      chName,
		Namespace: chNs,
	}
	testCases := []struct {
		desc   string
		srvCh  string
		chKey  string
		action string
		want   string
	}{
		{
			desc:   "empty servingChannel",
			srvCh:  "",
			chKey:  key.String(),
			action: "",
			want:   "",
		},
		{
			desc:   "empty action",
			srvCh:  "ch/a,ch/b",
			chKey:  key.String(),
			action: "",
			want:   "ch/a,ch/b",
		},
		{
			desc:   "adding to existing servingChannel",
			srvCh:  "ch/a,ch/b",
			chKey:  types.NamespacedName{Name: "a", Namespace: "ch"}.String(),
			action: "add",
			want:   "ch/a,ch/b",
		},
		{
			desc:   "adding a new channel",
			srvCh:  "ch/a,ch/b",
			chKey:  types.NamespacedName{Name: "c", Namespace: "ch"}.String(),
			action: "add",
			want:   "ch/a,ch/b,ch/c",
		},
		{
			desc:   "delete a none existing channel",
			srvCh:  "ch/a,ch/b",
			chKey:  types.NamespacedName{Name: "c", Namespace: "ch"}.String(),
			action: "remove",
			want:   "ch/a,ch/b",
		},
		{
			desc:   "delete an existing channel",
			srvCh:  "ch/a,ch/b",
			chKey:  types.NamespacedName{Name: "a", Namespace: "ch"}.String(),
			action: "remove",
			want:   "ch/b",
		},
	}

	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			got := utils.UpdateServingChannel(tC.srvCh, tC.chKey, tC.action)
			a, b := convertCommaStringToMap(got), convertCommaStringToMap(tC.want)
			if diff := cmp.Diff(a, b); diff != "" {
				t.Errorf("UpdateServingChannel mismatch (%v, %v)", tC.want, got)
			}
		})
	}
}

func convertCommaStringToMap(s string) map[string]bool {
	m := make(map[string]bool)

	if s == "" {
		return m
	}

	parsedstr := strings.Split(s, ",")

	for _, ch := range parsedstr {
		m[ch] = true
	}

	return m
}
