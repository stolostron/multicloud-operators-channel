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

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type ChannelValidator struct {
	client.Client
	decoder *admission.Decoder
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-apps-open-cluster-management-io-v1-channel,mutating=false,failurePolicy=fail,groups=apps.open-cluster-management.io.open-cluster-management.io,resources=channels,versions=v1,name=vchannel.kb.io

// ChannelValidator admits a channel if a specific channel can coexit in the
// requested namespace.
func (v *ChannelValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	chn := &chv1.Channel{}

	err := v.decoder.Decode(req, chn)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if chn.Spec.Type == chv1.ChannelTypeGit {
		return admission.Allowed("")
	}

	chList := &chv1.ChannelList{}
	if err := v.List(ctx, chList, client.InNamespace(chn.GetNamespace())); err != nil {
		return admission.Denied(fmt.Sprint("k8s cluster state unknow "))
	}

	if len(chList.Items) != 0 {
		return admission.Denied(fmt.Sprint("there's channel in the requested namespace"))
	}

	return admission.Allowed("")
}

// ChannelValidator implements admission.DecoderInjector.
// A decoder will be automatically injected.

// InjectDecoder injects the decoder.
func (v *ChannelValidator) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}
