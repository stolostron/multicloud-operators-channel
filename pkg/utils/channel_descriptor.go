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
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	chv1 "github.com/open-cluster-management/multicloud-operators-channel/pkg/apis/apps/v1"
)

// ChannelDescription contains channel and its object store information
type ChannelDescription struct {
	Channel *chv1.Channel
	Bucket  string
	ObjectStore
}

// ChannelDescriptor stores channel descriptions and object store connections
type ChannelDescriptor struct {
	sync.RWMutex
	channelUnitRegistry map[string]*ChannelDescription // key: channel name
}

func (desc *ChannelDescriptor) GetBucketNameByChannel(chName string) string {
	if _, ok := desc.channelUnitRegistry[chName]; !ok {
		return ""
	}

	return desc.channelUnitRegistry[chName].Bucket
}

// CreateChannelDescriptor - creates an instance of ChannelDescriptor
func CreateObjectStorageChannelDescriptor() (*ChannelDescriptor, error) {
	c := &ChannelDescriptor{
		channelUnitRegistry: make(map[string]*ChannelDescription),
	}

	return c, nil
}

//SetObjectStorageForChannel is mainly for testing purpose
func (desc *ChannelDescriptor) SetObjectStorageForChannel(chn *chv1.Channel, objStoreHandler ObjectStore) bool {
	if _, ok := desc.channelUnitRegistry[chn.Name]; !ok {
		desc.channelUnitRegistry[chn.Name] = &ChannelDescription{}
	}

	_, bucket := parseBucketAndEndpoint(chn.Spec.Pathname)
	t := desc.channelUnitRegistry[chn.Name]
	t.Channel = chn.DeepCopy()
	t.Bucket = bucket
	t.ObjectStore = objStoreHandler

	return true
}

// ConnectWithResourceHost validates and makes channel object store connection
func (desc *ChannelDescriptor) ConnectWithResourceHost(chn *chv1.Channel, kubeClient client.Client, log logr.Logger, objStoreHandler ...ObjectStore) error {
	var storageHanler ObjectStore

	chUnit, _ := desc.Get(chn.Name)

	// the objStoreHandler will be picked up by the following order,
	// 1, injected on of this func
	// 2, if not injected, and there's old one for the channel, then use the old one
	// 3, otherwise, use the AWS one
	if len(objStoreHandler) != 0 && objStoreHandler[0] != nil {
		storageHanler = objStoreHandler[0]
	} else if chUnit != nil && chUnit.ObjectStore != nil {
		storageHanler = chUnit.ObjectStore
	} else {
		storageHanler = &AWSHandler{}
	}

	var accessID, secretAccessKey string

	var err error

	if chn.Spec.SecretRef != nil {
		accessID, secretAccessKey, err = getCredentialFromKube(chn.Spec.SecretRef, chn.GetNamespace(), kubeClient)
		if err != nil {
			log.Error(err, "failed to fetch the reference secret")
			return err
		}
	}
	// Add new channel to the map
	if err := desc.updateChannelRegistry(chn, accessID, secretAccessKey, storageHanler, log); err != nil {
		log.Error(err, "unable to initialize channel ObjectStore description")
		return err
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

	accessKeyID, secretAccessKey = ParseSecertInfo(secret)
	return accessKeyID, secretAccessKey, nil
}

func parseBucketAndEndpoint(pathName string) (string, string) {
	if pathName == "" {
		return "", ""
	}

	if strings.HasSuffix(pathName, "/") {
		last := len(pathName) - 1
		pathName = pathName[:last]
	}

	loc := strings.LastIndex(pathName, "/")
	endpoint := pathName[:loc]
	bucket := pathName[loc+1:]

	return endpoint, bucket
}

func (desc *ChannelDescriptor) updateChannelRegistry(chn *chv1.Channel, accessKeyID, secretAccessKey string,
	objStoreHandler ObjectStore, log logr.Logger) error {
	chndesc := &ChannelDescription{}

	endpoint, bucket := parseBucketAndEndpoint(chn.Spec.Pathname)

	chndesc.Bucket = bucket

	log.Info(fmt.Sprintf("trying to connect to object bucket %v|%v", endpoint, chndesc.Bucket))

	if err := objStoreHandler.InitObjectStoreConnection(endpoint, accessKeyID, secretAccessKey); err != nil {
		log.Error(err, "unable to initialize object store settings")
		return err
	}
	// Check whether the connection is setup successfully
	if err := objStoreHandler.Exists(chndesc.Bucket); err != nil {
		log.Error(err, fmt.Sprint("unable to access object store bucket ", chndesc.Bucket, " for channel ", chn.Name))
		return err
	}

	chndesc.ObjectStore = objStoreHandler

	chndesc.Channel = chn.DeepCopy()

	desc.Put(chn.Name, chndesc)

	log.Info(fmt.Sprint("Channel ObjectStore descriptor for ", chn.Name, " is initialized: ", chndesc))

	return nil
}

// Get channel description for a channel
func (desc *ChannelDescriptor) Get(chname string) (chdesc *ChannelDescription, ok bool) {
	desc.RLock()
	result, ok := desc.channelUnitRegistry[chname]
	desc.RUnlock()

	return result, ok
}

// Delete the channel description
func (desc *ChannelDescriptor) Delete(chname string) {
	desc.Lock()
	delete(desc.channelUnitRegistry, chname)
	desc.Unlock()
}

// Put a new channel description
func (desc *ChannelDescriptor) Put(chname string, chdesc *ChannelDescription) {
	desc.Lock()
	desc.channelUnitRegistry[chname] = chdesc
	desc.Unlock()
}
