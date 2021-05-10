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

package controller

func init() {
	// stop object store channel controller, which is for syncing local objects back to online object bucket
	// There is no need to do this sync up as the onine object bucket is the only source of truth now.
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	//AddToManagerFuncs = append(AddToManagerFuncs, objectstore.Add)
}
