# multicloud-operators-channel

[![Build](https://travis-ci.com/open-cluster-management/multicloud-operators-channel.svg?branch=master)](https://travis-ci.com/open-cluster-management/multicloud-operators-channel.svg?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/open-cluster-management/multicloud-operators-channel)](https://goreportcard.com/report/github.com/open-cluster-management/multicloud-operators-channel)
[![GoDoc](https://godoc.org/github.com/open-cluster-management/multicloud-operators-channel?status.svg)](https://godoc.org/github.com/open-cluster-management/multicloud-operators-channel?status.svg)
[![Sonarcloud Status](https://sonarcloud.io/api/project_badges/measure?project=open-cluster-management_multicloud-operators-channel&metric=coverage)](https://sonarcloud.io/api/project_badges/measure?project=open-cluster-management_multicloud-operators-channel&metric=coverage)
[![License](https://img.shields.io/:license-apache-blue.svg)](http://www.apache.org/licenses/LICENSE-2.0.html)

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**  *generated with [DocToc](https://github.com/thlorenz/doctoc)*

- [multicloud-operators-application](#multicloud-operators-application)
    - [What is the multicloud-operators-application](#what-is-the-multicloud-operators-application)
    - [Getting started](#getting-started)
    - [References](#references)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## What is the multicloud-operators-channel

The `multicloud-operators-channel` controller syncs and promotes resources from the target source to a channel namespace on your hub cluster, or to a host, such as object bucket. Your subscription can then consume these resources from the channel directly.

Go to the [Contributing guide](CONTRIBUTING.md) to learn how to get involved.

## Getting started

- Clone and build an image with the [Development guide](docs/development.md).

- Check the [Security guide](SECURITY.md) if you need to report a security issue.

### Setting up a channel to sync resources between your hub cluster and an object bucket

Keep in mind, if you were using this operator before, you might need to migrate your resource api-group and version from `app.ibm.com/v1alpha1` to `apps.open-cluster-management.io/v1`. 

An example can be found under the `examples` folder.

To get started quickly, create a namespace channel. What does this mean? You need to assign the resource to a namespace, which is watched by our channel operator. The operator promotes this resource to the channel namespace, which should be the namespace the where the channel operator sits.

The following example is tested on a minikube so that you can play with this operator without worrying about things like security for a real cluster.

1. Clone the channel operator repository with the following command:

```shell
% mkdir -p "$GOPATH"/src/github.com/open-cluster-management

% cd "$GOPATH"/src/github.com/open-cluster-management

% git clone https://github.com/open-cluster-management/multicloud-operators-channel.git

% cd "$GOPATH"/src/github.com/open-cluster-management/multicloud-operators-channel
```

2. Set up the environment and deploy the channel operator:

```shell
# apply all the necessary CRDs

% kubectl apply -f ./deploy/crds
% kubectl apply -f ./deploy/dependent-crds

# deploy the channel controller to your cluster via a deployment, also grant access to the controller

% kubectl apply -f ./deploy/standalone
```

3. Create a channel and deploy the payload. The payload is the resource that you want the channel operator to help you move around or promote:

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

Resources got moved to it destination, and `subscription` can take over from here, deploying this resource (it will be the configmap deployed) to your managed cluster. 

```shell
% kubectl get deployables.apps.open-cluster-management.io -n ch-ns

NAME                                  TEMPLATE-KIND   TEMPLATE-APIVERSION   AGE   STATUS
payload-cfg-namespace-channel-gt47s   ConfigMap       v1                    37s
```

### Troubleshooting

Try the following methods if you need to troubleshoot:

- Check operator availability:

```shell
% kubectl get deploy,pods
NAME                                           READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/multicloud-operators-channel   1/1     1            1           2m1s

NAME                                                READY   STATUS    RESTARTS   AGE
pod/multicloud-operators-channel-7cbd9fbd55-kjkpc   1/1     Running   0          2m
```

- Check the channel and its status:

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
- Check the channel operator log. Usually you want to check the reconcile trace. For each flow, it should at least have a valid reconcile log for the deployable:

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

## References

### multicloud-operators repositories

- Access the following multicloud-operators repositories:

  - [multicloud-operators-application](https://github.com/open-cluster-management/multicloud-operators-application)
  - [multicloud-operators-channel](https://github.com/open-cluster-management/multicloud-operators-channel)
  - [multicloud-operators-deployable](https://github.com/open-cluster-management/multicloud-operators-deployable)
  - [multicloud-operators-placementrule](https://github.com/open-cluster-management/multicloud-operators-placementrule)
  - [multicloud-operators-subscription](https://github.com/open-cluster-management/multicloud-operators-subscription)
  - [multicloud-operators-subscription-release](https://github.com/open-cluster-management/multicloud-operators-subscription-release)
