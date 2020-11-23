// Copyright 2019 The Kubernetes Authors.
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
	MetricsAddr  string
	SyncInterval int
	LeaderElect  bool
	Debug        bool
}

var (
	options = ChannelCMDOptions{
		MetricsAddr:  "",
		SyncInterval: defaultSyncInterval,
		Debug:        false,
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
		"if debug is true, then webhooks will be created",
	)
}
