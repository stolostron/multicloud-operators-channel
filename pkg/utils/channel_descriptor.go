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
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"

	chnv1alpha1 "github.com/IBM/multicloud-operators-channel/pkg/apis/app/v1alpha1"
)

// ChannelDescription contains channel and its object store information
type ChannelDescription struct {
	Channel *chnv1alpha1.Channel
	Bucket  string
	ObjectStore
}

// ChannelDescriptor stores channel descriptions and object store connections
type ChannelDescriptor struct {
	sync.RWMutex
	channelDescriptorMap map[string]*ChannelDescription // key: channel name
}

// CreateChannelDescriptor - creates an instance of ChannelDescriptor
func CreateChannelDescriptor() (*ChannelDescriptor, error) {
	c := &ChannelDescriptor{
		channelDescriptorMap: make(map[string]*ChannelDescription),
	}

	return c, nil
}

// ValidateChannel validates and makes channel object store connection
func (desc *ChannelDescriptor) ValidateChannel(chn *chnv1alpha1.Channel, kubeClient client.Client, objStoreHandler ...ObjectStore) error {
	var storageHanler ObjectStore

	if len(objStoreHandler) == 0 {
		storageHanler = &AWSHandler{}
	} else {
		storageHanler = objStoreHandler[0]
	}

	chndesc, _ := desc.Get(chn.Name)

	accessId, secretAccessKey, err := getCredentialFromKube(chn.Spec.SecretRef, chn.GetNamespace(), kubeClient)
	if err != nil {
		klog.Error(err)
		return err
	}
	// Add new channel to the map
	if chndesc == nil {

		if err := desc.initChannelDescription(chn, accessId, secretAccessKey, storageHanler); err != nil {
			klog.Error(err, "unable to initialize channel ObjectStore description")
			return err
		}
	}
	// Check whether channel description and its object store connection are still valid
	err = chndesc.ObjectStore.Exists(chndesc.Bucket)
	if err != nil || !reflect.DeepEqual(chndesc.Channel, chn) {
		// remove invalid channel description
		desc.Delete(chn.Name)
		klog.Info("updating ObjectStore description for channel ", chn.Name)

		if err := desc.initChannelDescription(chn, accessId, secretAccessKey, chndesc.ObjectStore); err != nil {
			klog.Error(err, "Unable to initialize channel ObjectStore description")
			return err
		}
	}

	return nil
}

func getCredentialFromKube(secretRef *corev1.ObjectReference, defaultNs string, kubeClient client.Client) (string, string, error) {
	if secretRef == nil {
		return "", "", errors.New("failed to get access info to objectstore due to missing referred secret")
	}
	accessKeyID := ""
	secretAccessKey := ""

	secret := &corev1.Secret{}
	secns := secretRef.Namespace

	if secns == "" {
		secns = defaultNs
	}

	err := kubeClient.Get(context.TODO(), types.NamespacedName{Name: secretRef.Name, Namespace: secns}, secret)

	if err != nil {
		return "", "", errors.Wrap(err, "unable to get secret")
	}

	err = yaml.Unmarshal(secret.Data[SecretMapKeyAccessKeyID], &accessKeyID)
	if err != nil {
		return "", "", errors.Wrap(err, "unable to unmarshal secret")
	}

	err = yaml.Unmarshal(secret.Data[SecretMapKeySecretAccessKey], &secretAccessKey)
	if err != nil {
		return "", "", errors.Wrap(err, "unable to unmarshal secret")
	}
	return accessKeyID, secretAccessKey, nil
}

func (desc *ChannelDescriptor) initChannelDescription(chn *chnv1alpha1.Channel, accessKeyID, secretAccessKey string, objStoreHandler ObjectStore) error {
	chndesc := &ChannelDescription{}

	pathName := chn.Spec.PathName
	if pathName == "" {
		return errors.New(fmt.Sprintf("empty pathname in channel %v", chn.Name))
	}

	if strings.HasSuffix(pathName, "/") {
		last := len(pathName) - 1
		pathName = pathName[:last]
	}

	loc := strings.LastIndex(pathName, "/")
	endpoint := pathName[:loc]
	chndesc.Bucket = pathName[loc+1:]

	klog.Info("Trying to connect to aws ", endpoint, " | ", chndesc.Bucket)

	if err := objStoreHandler.InitObjectStoreConnection(endpoint, accessKeyID, secretAccessKey); err != nil {
		klog.Error(err, "unable to initialize object store settings")
		return err
	}
	// Check whether the connection is setup successfully
	if err := objStoreHandler.Exists(chndesc.Bucket); err != nil {
		klog.Error(err, "nable to access object store bucket ", chndesc.Bucket, " for channel ", chn.Name)
		return err
	}

	chndesc.ObjectStore = objStoreHandler

	chndesc.Channel = chn.DeepCopy()

	desc.Put(chn.Name, chndesc)

	klog.Info("Channel ObjectStore descriptor for ", chn.Name, " is initialized: ", chndesc)

	return nil
}

// Get channel description for a channel
func (desc *ChannelDescriptor) Get(chname string) (chdesc *ChannelDescription, ok bool) {
	desc.RLock()
	result, ok := desc.channelDescriptorMap[chname]
	desc.RUnlock()

	return result, ok
}

// Delete the channel description
func (desc *ChannelDescriptor) Delete(chname string) {
	desc.Lock()
	delete(desc.channelDescriptorMap, chname)
	desc.Unlock()
}

// Put a new channel description
func (desc *ChannelDescriptor) Put(chname string, chdesc *ChannelDescription) {
	desc.Lock()
	desc.channelDescriptorMap[chname] = chdesc
	desc.Unlock()
}
