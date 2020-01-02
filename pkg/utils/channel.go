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
	"context"

	"strings"

	"k8s.io/klog"

	appv1alpha1 "github.com/IBM/multicloud-operators-channel/pkg/apis/app/v1alpha1"
	dplutils "github.com/IBM/multicloud-operators-deployable/pkg/utils"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GenerateChannelMap finds all channels and build map with key of channel name
func GenerateChannelMap(cl client.Client) (map[string]*appv1alpha1.Channel, error) {
	if klog.V(10) {
		fnName := dplutils.GetFnName()
		klog.Infof("Entering: %v()", fnName)

		defer klog.Infof("Exiting: %v()", fnName)
	}
	// try to load channelmap if it is empty
	chlist := &appv1alpha1.ChannelList{}
	err := cl.List(context.TODO(), chlist, &client.ListOptions{})

	if err != nil {
		return nil, err
	}

	chmap := make(map[string]*appv1alpha1.Channel)

	for _, ch := range chlist.Items {
		klog.V(10).Infof("Channel namespacedname: %v/%v,  type: %v, sourceNamespaces: %v, gates: %#v",
			ch.Namespace, ch.Name, ch.Spec.Type, ch.Spec.SourceNamespaces, ch.Spec.Gates)
		chmap[ch.Name] = ch.DeepCopy()
	}

	return chmap, err
}

// LocateChannel finds channel by name
func LocateChannel(cl client.Client, name string) (*appv1alpha1.Channel, error) {
	if klog.V(10) {
		fnName := dplutils.GetFnName()
		klog.Infof("Entering: %v()", fnName)

		defer klog.Infof("Exiting: %v()", fnName)
	}
	// try to load channelmap if it is empty
	chlist := &appv1alpha1.ChannelList{}
	err := cl.List(context.TODO(), chlist, &client.ListOptions{})

	if err != nil {
		return nil, err
	}

	for _, ch := range chlist.Items {
		if ch.Name == name {
			return &ch, nil
		}
	}

	return nil, nil
}

// UpdateServingChannel add/remove the given channel to the current serving channel
func UpdateServingChannel(servingChannel string, channelKey string, action string) string {
	if klog.V(10) {
		fnName := dplutils.GetFnName()
		klog.Infof("Entering: %v()", fnName)

		defer klog.Infof("Exiting: %v()", fnName)
	}

	parsedstr := strings.Split(servingChannel, ",")

	newChannelMap := make(map[string]bool)

	for _, ch := range parsedstr {
		newChannelMap[ch] = true
	}

	if action == "remove" {
		_, ok := newChannelMap[channelKey]
		if ok {
			delete(newChannelMap, channelKey)
		}
	}

	if action == "add" {
		_, ok := newChannelMap[channelKey]
		if !ok {
			newChannelMap[channelKey] = true
		}
	}

	newChannelList := ""
	for newch := range newChannelMap {
		if newChannelList > "" {
			newChannelList = newChannelList + ","
		}

		newChannelList = newChannelList + newch
	}

	return newChannelList
}
