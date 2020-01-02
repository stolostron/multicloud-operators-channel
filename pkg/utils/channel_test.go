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
	"testing"

	"github.com/onsi/gomega"

	appv1alpha1 "github.com/IBM/multicloud-operators-channel/pkg/apis/app/v1alpha1"
	"github.com/IBM/multicloud-operators-channel/pkg/utils"
)

// this is mainly testing if a Channel resource can be created or not
func TestGenerateChannelMap(t *testing.T) {
	g := gomega.NewWithT(t)

	fetched := &appv1alpha1.Channel{}
	g.Expect(c.Get(context.TODO(), key, fetched)).NotTo(gomega.HaveOccurred())

	got, _ := utils.GenerateChannelMap(c)
	want := map[string]*appv1alpha1.Channel{chName: fetched}

	// test cases
	g.Expect(got[chName].Spec).To(gomega.Equal(want[chName].Spec))
	g.Expect(got[chName].ObjectMeta).To(gomega.Equal(want[chName].ObjectMeta))
}

func TestLocateChannel(t *testing.T) {
	g := gomega.NewWithT(t)
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
