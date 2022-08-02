// Copyright 2021 The Kubernetes Authors.
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

package utils

import (
	"testing"

	"github.com/onsi/gomega"
)

func TestCreate(t *testing.T) {
	testBucket := "helloWorld"
	testObj := "my object"

	var storageHandler = &AWSHandler{}

	g := gomega.NewGomegaWithT(t)

	err := storageHandler.InitObjectStoreConnection("", SecretMapKeyAccessKeyID, SecretMapKeySecretAccessKey, SecretMapKeyRegion)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Fail to create bucket
	err = storageHandler.Create(testBucket)
	g.Expect(err).To(gomega.HaveOccurred())

	// Fail to get existing object
	dplObj, err := storageHandler.Get(testBucket, testObj)
	g.Expect(err).To(gomega.HaveOccurred())
	g.Expect(dplObj).To(gomega.Equal(DeployableObject{}))

	// Empty deployableObject
	err = storageHandler.Put(testBucket, DeployableObject{})
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// non-empty deployableObject
	err = storageHandler.Put(testBucket, DeployableObject{Name: "my deployable object"})
	g.Expect(err).To(gomega.HaveOccurred())

	// Fail to delete object
	err = storageHandler.Delete(testBucket, testObj)
	g.Expect(err).To(gomega.HaveOccurred())

	// Fail to retrieve all objects
	objects, err := storageHandler.List(testBucket)
	g.Expect(err).To(gomega.HaveOccurred())
	g.Expect(objects).To(gomega.BeNil())
}
