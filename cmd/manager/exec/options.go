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

package exec

import (
	pflag "github.com/spf13/pflag"
)

// PlacementRuleCMDOptions for command line flag parsing
type ChannelCMDOptions struct {
	MetricsAddr           string
	CRDPathName           string
	DeployableCRDPathName string
	SyncInterval          int
	LeaderElect           bool
}

var options = ChannelCMDOptions{
	MetricsAddr:           "",
	SyncInterval:          60,
	CRDPathName:           "/usr/local/etc/channel/crds/app_v1alpha1_channel.yaml",
	DeployableCRDPathName: "/usr/local/etc/deployable/crds/app_v1alpha1_deployable.yaml",
}

// ProcessFlags parses command line parameters into options
func ProcessFlags() {
	flag := pflag.CommandLine
	// add flags
	flag.StringVar(
		&options.MetricsAddr,
		"metrics-addr",
		options.MetricsAddr,
		"The address the metric endpoint binds to.",
	)

	flag.StringVar(
		&options.DeployableCRDPathName,
		"deployable-crd-pathname",
		options.DeployableCRDPathName,
		"The pathname of deployable crd file",
	)

	flag.StringVar(
		&options.CRDPathName,
		"channel-crd-pathname",
		options.CRDPathName,
		"The pathname of channel crd file",
	)

	flag.IntVar(
		&options.SyncInterval,
		"sync-interval",
		options.SyncInterval,
		"Setting up the cache sync time.",
	)

	flag.BoolVar(
		&options.LeaderElect,
		"leader-elect",
		false,
		"Enable a leader client to gain leadership before executing the main loop")
}
