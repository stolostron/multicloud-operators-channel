// Licensed Materials - Property of IBM
// (c) Copyright IBM Corporation 2016, 2019. All Rights Reserved.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP  Schedule Contract with IBM Corp.

package utils

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"

	chnv1alpha1 "github.com/IBM/multicloud-operators-channel/pkg/apis/app/v1alpha1"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
func (desc *ChannelDescriptor) ValidateChannel(chn *chnv1alpha1.Channel, kubeClient client.Client) error {
	chndesc, _ := desc.Get(chn.Name)

	// Add new channel to the map
	if chndesc == nil {
		if err := desc.initChannelDescription(chn, kubeClient); err != nil {
			klog.Error(err, "Unable to initialize channel ObjectStore description")
			return err
		}
	} else {
		// Check whether channel description and its object store connection are still valid
		err := chndesc.ObjectStore.Exists(chndesc.Bucket)
		if err != nil || !reflect.DeepEqual(chndesc.Channel, chn) {
			// remove invalid channel description
			desc.Delete(chn.Name)
			klog.Info("Updating ObjectStore description for channel ", chn.Name)
			if err := desc.initChannelDescription(chn, kubeClient); err != nil {
				klog.Error(err, "Unable to initialize channel ObjectStore description")
				return err
			}
		}
	}

	return nil
}

func (desc *ChannelDescriptor) initChannelDescription(chn *chnv1alpha1.Channel, kubeClient client.Client) error {
	chndesc := &ChannelDescription{}

	pathName := chn.Spec.PathName
	if pathName == "" {
		errmsg := "Empty Pathname in channel " + chn.Name
		klog.Error(errmsg)
		return errors.New(errmsg)
	}
	if strings.HasSuffix(pathName, "/") {
		last := len(pathName) - 1
		pathName = pathName[:last]
	}
	loc := strings.LastIndex(pathName, "/")
	endpoint := pathName[:loc]
	chndesc.Bucket = pathName[loc+1:]

	klog.Info("Trying to connect to aws ", endpoint, " | ", chndesc.Bucket)

	awshandler := &AWSHandler{}
	accessKeyID := ""
	secretAccessKey := ""
	if chn.Spec.SecretRef != nil {
		secret := &corev1.Secret{}
		secns := chn.Spec.SecretRef.Namespace
		if secns == "" {
			secns = chn.Namespace
		}
		err := kubeClient.Get(context.TODO(), types.NamespacedName{Name: chn.Spec.SecretRef.Name, Namespace: secns}, secret)
		if err != nil {
			klog.Error(err, "Unable to get secret")
			return err
		}

		yaml.Unmarshal(secret.Data[SecretMapKeyAccessKeyID], &accessKeyID)
		yaml.Unmarshal(secret.Data[SecretMapKeySecretAccessKey], &secretAccessKey)
	}
	if err := awshandler.InitObjectStoreConnection(endpoint, accessKeyID, secretAccessKey); err != nil {
		klog.Error(err, "Unable to initialize object store settings")
		return err
	}
	// Check whether the connection is setup successfully
	if err := awshandler.Exists(chndesc.Bucket); err != nil {
		klog.Error(err, "Unable to access object store bucket ", chndesc.Bucket, " for channel ", chn.Name)
		return err
	}
	chndesc.ObjectStore = awshandler

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
