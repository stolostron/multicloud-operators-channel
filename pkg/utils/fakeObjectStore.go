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

package utils

import (
	"github.com/pkg/errors"
)

type FakeObjectStore struct {
	//map[bucket]map[objName]DeployableObject[Name, Content]
	Clt map[string]map[string]DeployableObject
}

func (m *FakeObjectStore) InitObjectStoreConnection(endpoint, accessKeyID, secretAccessKey string) error {
	if len(m.Clt) == 0 {
		m.Clt = make(map[string]map[string]DeployableObject)
	}

	return nil
}

//it's odd that we request the storage to be pre-set
func (m *FakeObjectStore) Exists(bucket string) error {
	if _, ok := m.Clt[bucket]; !ok {
		return m.Create(bucket)
	}

	return nil
}

func (m *FakeObjectStore) Create(bucket string) error {
	m.Clt[bucket] = make(map[string]DeployableObject)

	return nil
}

func (m *FakeObjectStore) List(bucket string) ([]string, error) {
	keys := []string{}

	for k := range m.Clt[bucket] {
		keys = append(keys, k)
	}

	return keys, nil
}

func (m *FakeObjectStore) Put(bucket string, dplObj DeployableObject) error {
	m.Clt[bucket] = map[string]DeployableObject{
		dplObj.Name: dplObj,
	}

	return nil
}

func (m *FakeObjectStore) Delete(bucket, name string) error {
	if _, ok := m.Clt[bucket]; !ok {
		return errors.New("empty bucket")
	}

	delete(m.Clt, bucket)

	return nil
}

func (m *FakeObjectStore) Get(bucket, name string) (DeployableObject, error) {
	if _, ok := m.Clt[bucket][name]; !ok {
		return DeployableObject{}, errors.New("empty bucket")
	}

	return m.Clt[bucket][name], nil
}
