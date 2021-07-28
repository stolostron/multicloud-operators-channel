# multicloud-operators-channel

[![Build](https://travis-ci.com/open-cluster-management/multicloud-operators-channel.svg?branch=main)](https://travis-ci.com/open-cluster-management/multicloud-operators-channel.svg?branch=main)
[![Go Report Card](https://goreportcard.com/badge/github.com/open-cluster-management/multicloud-operators-channel)](https://goreportcard.com/report/github.com/open-cluster-management/multicloud-operators-channel)
[![GoDoc](https://godoc.org/github.com/open-cluster-management/multicloud-operators-channel?status.svg)](https://godoc.org/github.com/open-cluster-management/multicloud-operators-channel?status.svg)
[![Sonarcloud Status](https://sonarcloud.io/api/project_badges/measure?project=open-cluster-management_multicloud-operators-channel&metric=coverage)](https://sonarcloud.io/api/project_badges/measure?project=open-cluster-management_multicloud-operators-channel&metric=coverage)
[![License](https://img.shields.io/:license-apache-blue.svg)](http://www.apache.org/licenses/LICENSE-2.0.html)

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**  *generated with [DocToc](https://github.com/thlorenz/doctoc)*

- [Overview](#overview)
- [Quick Start](#quick-start)
  - [Setting up a channel to sync resourcese between your hub cluster and a object bucket](#setting-up-a-channel-to-sync-resourcese-between-your-hub-cluster-and-a-object-bucket)
  - [Trouble shooting](#trouble-shooting)
- [Community, discussion, contribution, and support](#community-discussion-contribution-and-support)
- [Security Response](#security-response)
- [References](#references)
  - [multicloud-operators repositories](#multicloud-operators-repositories)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->
## Overview

Channel controller sync and promote resources from the target source to a channel namespace on your hub cluster or to a host(such as object bucket). Then your subscription can consume these resources from the channel directly.

## Quick Start

Keep in mind, if you were using this operator before, you might need to migrate your resource api-group and version from `app.ibm.com/v1alpha1` to `apps.open-cluster-management.io/v1`. Example can be found under the `examples` folder.

For a quick start, we are going to create a namespace channel, meaning you can put the resouce to a namespace, which is watched by our channel operator. Then, the operator will promote this resoucre to the channel namespace, which should be the namesapce the channel operator sits.

The following example is tested on a minikube, so that you can play with this operator with out worrying too much extra stuff, such as secrity for a real cluster.

------

### Setting up a channel to sync resourcese between your hub cluster and a object bucket

- Clone the channel operator repository


```shell
% mkdir -p "$GOPATH"/src/github.com/open-cluster-management

% cd "$GOPATH"/src/github.com/open-cluster-management

% git clone https://github.com/open-cluster-management/multicloud-operators-channel.git

% cd "$GOPATH"/src/github.com/open-cluster-management/multicloud-operators-channel
```

- Setup environment and deploy channel operator

```shell
# apply all the necessary CRDs

% kubectl apply -f ./deploy/crds
% kubectl apply -f ./deploy/dependent-crds

# deploy the channel controller to your cluster via a deployment, also grant access to the controller

% kubectl apply -f ./deploy/standalone
```

- Create a Channel and deploy the payload(the resource you want channel operator to help you move/promote around)

```shell
# a simple namespace type channel example
% kubectl apply -f ./examples/channel-alone
```

As a result, a config map wrapped by `deployable`,  at default namespace. At the meantime, it will also deploy a `channel` resource at the `ch-ns` namespace.


```
% kubectl get deployables.apps.open-cluster-management.io

NAME                            TEMPLATE-KIND   TEMPLATE-APIVERSION   AGE   STATUS
payload-cfg-namespace-channel   ConfigMap       v1                    9s

```

```
% kubectl get channel -n ch-ns

NAME   TYPE        PATHNAME   AGE
ns     Namespace   ch-ns      20s
```

Resouces got moved to it destination, and `subscription` can take over from here, deploying this resource(it will be the configmap deployed) to your managed cluster. 

```shell
% kubectl get deployables.apps.open-cluster-management.io -n ch-ns

NAME                                  TEMPLATE-KIND   TEMPLATE-APIVERSION   AGE   STATUS
payload-cfg-namespace-channel-gt47s   ConfigMap       v1                    37s
```

### Trouble shooting

- Check operator availability

```shell
% kubectl get deploy,pods
NAME                                           READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/multicloud-operators-channel   1/1     1            1           2m1s

NAME                                                READY   STATUS    RESTARTS   AGE
pod/multicloud-operators-channel-7cbd9fbd55-kjkpc   1/1     Running   0          2m
```

- Check Channel and its status

```shell
% kubectl describe channel ns -n ch-ns

Name:         ns
Namespace:    ch-ns
Labels:       <none>
Annotations:  kubectl.kubernetes.io/last-applied-configuration:
                {"apiVersion":"apps.open-cluster-management.io/v1","kind":"Channel","metadata":{"annotations":{},"name":"ns","namespace":"ch-ns"},"spec":{...
API Version:  apps.open-cluster-management.io/v1
Kind:         Channel
Metadata:
  Creation Timestamp:  2020-03-05T02:03:32Z
  Generation:          1
  Resource Version:    861
  Self Link:           /apis/apps.open-cluster-management.io/v1/namespaces/ch-ns/channels/ns
  UID:                 fbec06b9-eedf-4fe0-8314-2e195e9bc192
Spec:
  Pathname:  ch-ns
  Source Namespaces:
    default
  Type:  Namespace
Events:
  Type    Reason  Age   From     Message
  ----    ------  ----  ----     -------
  Normal  Deploy  117s  channel  Depolyable ch-ns/payload-cfg-namespace-channel-kglr8 created in the channel, Status: Success, Channel: ch-ns/ns

```
- Check Channel operator log

Normally, we care about the reconcile trace. For each flow, it should at least have a valid reconcile log for the deployable.

```shell
% kubectl logs multicloud-operators-channel-f4fbbb9d9-6mcql

I0225 15:47:46.659919       1 manager.go:65] Go Version: go1.13
I0225 15:47:46.660006       1 manager.go:66] Go OS/Arch: linux/amd64
I0225 15:47:46.660012       1 manager.go:67] Version of operator-sdk: v0.12.0
I0225 15:47:48.705442       1 manager.go:148] Registering Components.
I0225 15:47:48.705517       1 manager.go:151] setting up scheme
I0225 15:47:48.706721       1 manager.go:171] Setting up controller
I0225 15:47:48.707552       1 manager.go:178] setting up webhooks
time="2020-02-25T15:47:50Z" level=info msg="Could not create ServiceMonitor objecterrorno ServiceMonitor registered with the API" source="manager.go:216"
I0225 15:47:50.748181       1 manager.go:220] Install prometheus-operator in your cluster to create ServiceMonitor objectserrorno ServiceMonitor registered with the API
I0225 15:47:50.748663       1 manager.go:224] Starting the Cmd.
I0225 15:47:50.850112       1 synchronizer.go:78] Housekeeping loop ...
I0225 15:47:50.850377       1 synchronizer.go:125] Housekeeping loop ...
I0225 15:48:00.850993       1 synchronizer.go:78] Housekeeping loop ...
I0225 15:48:00.851535       1 synchronizer.go:125] Housekeeping loop ...
I0225 15:48:10.851468       1 synchronizer.go:78] Housekeeping loop ...
I0225 15:48:10.851723       1 synchronizer.go:125] Housekeeping loop ...
I0225 15:48:20.852381       1 synchronizer.go:125] Housekeeping loop ...
I0225 15:48:20.852529       1 synchronizer.go:78] Housekeeping loop ...

....


I0225 16:00:19.424095       1 deployable_controller.go:293] Creating deployable in channel{{ } { payload-cfg-namespace-channel- ch-ns    0 0001-01-01 00:00:00 +0000 UTC <nil> <nil> map[] map[app.ibm.com/channel:ch-ns/ns app.ibm.com/hosting-deployable:default/payload-cfg-namespace-channel app.ibm.com/is-local-deployable:false kubectl.kubernetes.io/last-applied-configuration:{"apiVersion":"multicloud-apps.io/v1alpha1","kind":"Deployable","metadata":{"annotations":{"app.ibm.com/is-local-deployable":"false"},"name":"payload-cfg-namespace-channel","namespace":"default"},"spec":{"channels":["ns"],"template":{"apiVersion":"v1","data":{"database":"mongodb"},"kind":"ConfigMap","metadata":{"name":"cfg-from-ch-qa"}}}}

```

## Community, discussion, contribution, and support

Check the [DEVELOPMENT Doc](docs/development.md) for how to build and make changes.

Check the [CONTRIBUTING Doc](CONTRIBUTING.md) for how to contribute to the repo.

You can reach the maintainers of this by raising issues. Slack communication is coming soon

## Security Response

Check the [Security Doc](SECURITY.md) if you've found a security issue. 

## References

### multicloud-operators repositories

- [multicloud-operators-application](https://github.com/open-cluster-management/multicloud-operators-application)
- [multicloud-operators-channel](https://github.com/open-cluster-management/multicloud-operators-channel)
- [multicloud-operators-deployable](https://github.com/open-cluster-management/multicloud-operators-deployable)
- [multicloud-operators-placementrule](https://github.com/open-cluster-management/multicloud-operators-placementrule)
- [multicloud-operators-subscription](https://github.com/open-cluster-management/multicloud-operators-subscription)
- [multicloud-operators-subscription-release](https://github.com/open-cluster-management/multicloud-operators-subscription-release)
