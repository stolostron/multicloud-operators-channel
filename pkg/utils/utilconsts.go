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

const (
	// HelmCRKind is kind of the Helm CR
	HelmCRKind = "HelmRelease"
	// SubscriptionCRKind is kind of the Subscription CR
	SubscriptionCRKind = "Subscription"
	// HelmCRAPIVersion is APIVersion of the Helm CR
	HelmCRAPIVersion = "apps.open-cluster-management.io/v1"
	// HelmCRChartName is spec.ChartName of the Helm CR
	HelmCRChartName = "chartName"
	// HelmCRReleaseName is spec.ReleaseName of the Helm CR
	HelmCRReleaseName = "releaseName"
	// HelmCRVersion is spec.Version of the Helm CR
	HelmCRVersion = "version"
	// HelmCRSource is spec.Source of the Helm CR
	HelmCRSource = "source"
	// HelmCRSourceType is spec.Source.Type of the Helm CR
	HelmCRSourceType = "type"
	// HelmCRSourceHelm is spec.Source.Helmrepo of the Helm CR
	HelmCRSourceHelm = "helmrepo"
	// HelmCRSourceGit is spec.Source.Git of the Helm CR
	HelmCRSourceGit = "git"
	// HelmCRRepoURL is spec.Source.Git.Urls or spec.Source.Helmrepo.Urls of the Helm CR
	HelmCRRepoURL = "urls"
	// HelmCRGitRepoChartPath is spec.Source.Github.ChartPath of the Helm CR
	HelmCRGitRepoChartPath = "chartPath"

	//Channel type meta for testing case
	ChannelTypeKind       = "Channel"
	ChannelTypeAPIVersion = "v1"

	//Deployable type meta for testing case
	DeployableTypeKind       = "Deployable"
	DeployableTypeAPIVersion = "v1"
)
