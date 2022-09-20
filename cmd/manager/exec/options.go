// Copyright 2021 The Kubernetes Authors.
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

const defaultSyncInterval = 60 //seconds

// ChannelCMDOptions for command line flag parsing
type ChannelCMDOptions struct {
	MetricsAddr                        string
	KubeConfig                         string
	SyncInterval                       int
	LeaderElect                        bool
	Debug                              bool
	LogLevel                           bool
	LeaderElectionLeaseDurationSeconds int
	RenewDeadlineSeconds               int
	RetryPeriodSeconds                 int
}

var (
	options = ChannelCMDOptions{
		MetricsAddr:                        "",
		SyncInterval:                       defaultSyncInterval,
		Debug:                              false,
		LogLevel:                           false,
		LeaderElectionLeaseDurationSeconds: 137,
		RenewDeadlineSeconds:               107,
		RetryPeriodSeconds:                 26,
	}
)

func HidKlogFlag(pf *pflag.FlagSet) {
	fStr := []string{
		"log_backtrace_at",
		"add_dir_header",
		"log_dir",
		"log_file_max_size",
		"log_file",
		"skip_headers",
		"skip_log_headers",
		"stderrthreshold",
		"vmodule",
		"alsologtostderr",
	}

	for _, f := range fStr {
		t := pf.Lookup(f)
		if t != nil {
			t.Hidden = true
		}
	}
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
		&options.KubeConfig,
		"kubeconfig",
		options.KubeConfig,
		"The kube config that points to an external api server.",
	)

	flag.IntVar(
		&options.SyncInterval,
		"sync-interval",
		options.SyncInterval,
		"Setting up the cache sync time.",
	)

	flag.BoolVar(
		&options.Debug,
		"debug",
		false,
		"if debug is true, then local webhooks will not be created",
	)

	flag.BoolVar(
		&options.LogLevel,
		"zap-devel",
		false,
		"zap-devel, default only log INFO(fasle), set to true for debugging",
	)

	flag.IntVar(
		&options.LeaderElectionLeaseDurationSeconds,
		"leader-election-lease-duration",
		options.LeaderElectionLeaseDurationSeconds,
		"The leader election lease duration in seconds.",
	)

	flag.IntVar(
		&options.RenewDeadlineSeconds,
		"renew-deadline",
		options.RenewDeadlineSeconds,
		"The renew deadline in seconds.",
	)

	flag.IntVar(
		&options.RetryPeriodSeconds,
		"retry-period",
		options.RetryPeriodSeconds,
		"The retry period in seconds.",
	)
}
