// Licensed Materials - Property of IBM
// (c) Copyright IBM Corporation 2016, 2019. All Rights Reserved.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP  Schedule Contract with IBM Corp.

package controller

import (
	gitsync "github.com/IBM/multicloud-operators-channel/pkg/synchronizer/githubsynchronizer"
	helmsync "github.com/IBM/multicloud-operators-channel/pkg/synchronizer/helmreposynchronizer"
	"github.com/IBM/multicloud-operators-channel/pkg/utils"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// AddToManagerFuncs is a list of functions to add all Controllers to the Manager
var AddToManagerFuncs []func(manager.Manager, record.EventRecorder, *utils.ChannelDescriptor, *helmsync.ChannelSynchronizer, *gitsync.ChannelSynchronizer) error

// AddToManager adds all Controllers to the Manager
func AddToManager(m manager.Manager, recorder record.EventRecorder, chdesc *utils.ChannelDescriptor, sync *helmsync.ChannelSynchronizer, gsync *gitsync.ChannelSynchronizer) error {

	for _, f := range AddToManagerFuncs {
		if err := f(m, recorder, chdesc, sync, gsync); err != nil {
			return err
		}
	}
	return nil
}
