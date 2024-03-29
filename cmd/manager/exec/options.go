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
	"time"

	pflag "github.com/spf13/pflag"
)

const defaultSyncInterval = 60 //seconds

// ChannelCMDOptions for command line flag parsing
type ChannelCMDOptions struct {
	MetricsAddr                 string
	KubeConfig                  string
	SyncInterval                int
	LeaderElect                 bool
	Debug                       bool
	LogLevel                    bool
	LeaderElectionLeaseDuration time.Duration
	LeaderElectionRenewDeadline time.Duration
	LeaderElectionRetryPeriod   time.Duration
}

var (
	options = ChannelCMDOptions{
		MetricsAddr:                 "",
		SyncInterval:                defaultSyncInterval,
		Debug:                       false,
		LogLevel:                    false,
		LeaderElectionLeaseDuration: 137 * time.Second,
		LeaderElectionRenewDeadline: 107 * time.Second,
		LeaderElectionRetryPeriod:   26 * time.Second,
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

	flag.DurationVar(
		&options.LeaderElectionLeaseDuration,
		"leader-election-lease-duration",
		options.LeaderElectionLeaseDuration,
		"The duration that non-leader candidates will wait after observing a leadership "+
			"renewal until attempting to acquire leadership of a led but unrenewed leader "+
			"slot. This is effectively the maximum duration that a leader can be stopped "+
			"before it is replaced by another candidate. This is only applicable if leader "+
			"election is enabled.",
	)

	flag.DurationVar(
		&options.LeaderElectionRenewDeadline,
		"leader-election-renew-deadline",
		options.LeaderElectionRenewDeadline,
		"The interval between attempts by the acting master to renew a leadership slot "+
			"before it stops leading. This must be less than or equal to the lease duration. "+
			"This is only applicable if leader election is enabled.",
	)

	flag.DurationVar(
		&options.LeaderElectionRetryPeriod,
		"leader-election-retry-period",
		options.LeaderElectionRetryPeriod,
		"The duration the clients should wait between attempting acquisition and renewal "+
			"of a leadership. This is only applicable if leader election is enabled.",
	)
}
