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

package webhook

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/go-logr/logr"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"
)

type ChannelValidator struct {
	logr.Logger
	client.Client
	decoder *admission.Decoder
}

//ValidateLogic add ChannelValidator to webhook wireup
var ValidateLogic = func(w *WireUp) {
	w.Handler = &ChannelValidator{Client: w.mgr.GetClient(), Logger: w.Logger}
}

// ChannelValidator admits a channel if a specific channel can co-exit in the
// requested namespace.
func (v *ChannelValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	v.Logger.Info("entry webhook handle")
	defer v.Logger.Info("exit webhook handle")

	chn := &chv1.Channel{}

	err := v.decoder.Decode(req, chn)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	chnType := string(chn.Spec.Type)
	if strings.EqualFold(chnType, chv1.ChannelTypeGit) || strings.EqualFold(chnType, chv1.ChannelTypeGitHub) {
		return admission.Allowed("")
	}

	chList := &chv1.ChannelList{}
	if err := v.List(ctx, chList, client.InNamespace(chn.GetNamespace())); err != nil {
		return admission.Denied(fmt.Sprint("the hub cluster state is unknown"))
	}

	if len(chList.Items) == 0 {
		return admission.Allowed("")
	}

	if v, ok := isAllGit(chList); !ok {
		return admission.Denied(fmt.Sprintf("the namespace %v already contains a channel: %v",
			chn.GetNamespace(), v))
	}

	return admission.Allowed("")
}

func isAllGit(chList *chv1.ChannelList) (string, bool) {
	for _, chn := range chList.Items {
		chnType := string(chn.Spec.Type)
		if strings.EqualFold(chnType, chv1.ChannelTypeGit) || strings.EqualFold(chnType, chv1.ChannelTypeGitHub) {
			continue
		}

		return chn.GetName(), false
	}

	return "", true
}

// ChannelValidator implements admission.DecoderInjector.
// A decoder will be automatically injected.

// InjectDecoder injects the decoder.
func (v *ChannelValidator) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}
