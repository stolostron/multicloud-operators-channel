# multicloud-operators-channel

[![Build](http://35.227.205.240/badge.svg?jobs=images_multicloud-operators-channel_postsubmit)](http://35.227.205.240/badge.svg?jobs=images_multicloud-operators-channel_postsubmit)
[![GoDoc](https://godoc.org/github.com/IBM/multicloud-operators-channel?status.svg)](https://godoc.org/github.com/IBM/multicloud-operators-channel)
[![Go Report Card](https://goreportcard.com/badge/github.com/IBM/multicloud-operators-channel)](https://goreportcard.com/report/github.com/IBM/multicloud-operators-channel)
[![Code Coverage](https://codecov.io/gh/IBM/multicloud-operators-channel/branch/master/graphs/badge.svg?branch=master)](https://codecov.io/gh/IBM/multicloud-operators-channel?branch=master)
[![License](https://img.shields.io/:license-apache-blue.svg)](http://www.apache.org/licenses/LICENSE-2.0.html)

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**  *generated with [DocToc](https://github.com/thlorenz/doctoc)*

- [Overview](#overview)
- [Quick Start](#quick-start)
  - [Setting up a channel to sync resourcese between your hub cluster and a object bucket](#setting-up-a-channel-to-sync-resourcese-between-your-hub-cluster-and-a-object-bucket)
  - [Trouble shooting](#trouble-shooting)
- [Community, discussion, contribution, and support](#community-discussion-contribution-and-support)
- [References](#references)
  - [multicloud-operators repositories](#multicloud-operators-repositories)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->
## Overview

------

Channel controller sync and promote resources from the target source to a channel namespace on your hub cluster or a host(such as object bucket). Then your subscription can consume these resources.

## Quick Start

------

### Setting up a channel to sync resourcese between your hub cluster and a object bucket

- Clone the subscription operator repository

```shell
% mkdir -p "$GOPATH"/src/github.com/open-cluster-management

% cd "$GOPATH"/src/github.com/open-cluster-management

% git clone https://github.com/open-cluster-management/multicloud-operators-subscription.git

% cd "$GOPATH"/src/github.com/open-cluster-management/multicloud-operators-subscription
```

- Setup environment and deploy subscription operator

```shell
# apply all the necessary CRDs
% kubectl apply -f ./deploy/crds

# deploy the channel controller to your cluster via a deployment, also grant access to the controller
% kubectl apply -f ./deploy/standalone
```

- Create a Channel and Subscription

```shell
# a simple namespace type channel example
% kubectl apply -f ./examples/channel-alone
```

- Subscribe!

```shell
$ kubectl patch subscriptions.app.ibm.com simple --type='json' -p='[{"op": "replace", "path": "/spec/placement/local", "value": true}]'
```

Find the nginx pods deployed to current namespace, and the number of backend pods is overrided to 3

```shell
% kubectl get pods -l app=nginx-ingress
NAME                                             READY   STATUS    RESTARTS   AGE
nginx-ingress-controller-857f44797-7fx7c         1/1     Running   0          96s
nginx-ingress-default-backend-6b8dc9d88f-97pxz   1/1     Running   0          96s
nginx-ingress-default-backend-6b8dc9d88f-drt7c   1/1     Running   0          96s
nginx-ingress-default-backend-6b8dc9d88f-n26ls   1/1     Running   0          96s
```

Check the [Getting Started](docs/getting_started.md) doc for more details

### Trouble shooting

- Check operator availability

```shell
% kubectl get deploy,pods
NAME                                                READY     UP-TO-DATE   AVAILABLE   AGE
deployment.apps/multicloud-operators-subscription   1/1       1            1           99m

NAME                                                     READY     STATUS    RESTARTS   AGE
pod/multicloud-operators-subscription-557c676479-dh2fg   1/1       Running   0          24s
```

- Check Subscription and its status

```shell
% kubectl describe appsub simple
Name:         simple
Namespace:    default
Labels:       <none>
Annotations:  kubectl.kubernetes.io/last-applied-configuration:
                {"apiVersion":"app.ibm.com/v1alpha1","kind":"Subscription","metadata":{"annotations":{},"name":"simple","namespace":"default"},"spec":{"ch...
API Version:  app.ibm.com/v1alpha1
Kind:         Subscription
Metadata:
  Creation Timestamp:  2019-11-21T04:01:47Z
  Generation:          2
  Resource Version:    24045
  Self Link:           /apis/app.ibm.com/v1alpha1/namespaces/default/subscriptions/simple
  UID:                 a35b6ef5-0c13-11ea-b4e7-00000a100ef8
Spec:
  Channel:  dev/dev-helmrepo
  Name:     nginx-ingress
  Package Overrides:
    Package Alias:  nginx-ingress-alias
    Package Name:  nginx-ingress
    Package Overrides:
      Path:   spec.values
      Value:  defaultBackend:
  replicaCount: 3

  Placement:
    Local:  true
Status:
  Last Update Time:  2019-11-21T04:02:38Z
  Phase:             Subscribed
  Statuses:
    /:
      Packages:
        dev-helmrepo-nginx-ingress-1.25.0:
          Last Update Time:  2019-11-21T04:02:38Z
          Phase:             Subscribed
          Resource Status:
            Last Update:  2019-11-21T04:02:24Z
            Phase:        Success
Events:                   <none>
```

Please refer to [Trouble shooting documentation](docs/trouble_shooting.md) for further info.

## Community, discussion, contribution, and support

------

Check the [DEVELOPMENT Doc](docs/development.md) for how to build and make changes.

Check the [CONTRIBUTING Doc](CONTRIBUTING.md) for how to contribute to the repo.

You can reach the maintainers of this by raising issues. Slack communication is coming soon

## References

------

### multicloud-operators repositories

- [multicloud-operators-deployable](https://github.com/open-cluster-management/multicloud-operators-deployable)
- [multicloud-operators-placementrule](https://github.com/open-cluster-management/multicloud-operators-placementrule)
- [multicloud-operators-channel](https://github.com/open-cluster-management/multicloud-operators-channel)
- [multicloud-operators-subscription](https://github.com/open-cluster-management/multicloud-operators-subscription)
- [multicloud-operators-subscription-release](https://github.com/open-cluster-management/multicloud-operators-subscription-release)

------

If you have any further questions, please refer to
[help documentation](docs/help.md) for further information.
