#!/bin/bash

#
# Copyright 2019 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


###!!!!!!!! On travis this script is run on the .git level
echo "E2E TESTS GO HERE!"

command -v
# need to find a way to use the Makefile to set these
REGISTRY=quay.io/open-cluster-management
IMG=$(cat COMPONENT_NAME 2> /dev/null)
IMAGE_NAME=${REGISTRY}/${IMG}
BUILD_IMAGE=${IMAGE_NAME}:latest

if ! command -v kubectl &> /dev/null; then
    echo "get kubectl binary"
    # Download and install kubectl
    curl -LO https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl && chmod +x kubectl && sudo mv kubectl /usr/local/bin/
    if [ $? != 0 ]; then
        exit $?;
    fi
fi

if ! command -v kind &> /dev/null; then
    # Download and install KinD
    GO111MODULE=on go get sigs.k8s.io/kind
fi

if [ "$TRAVIS_BUILD" != 1 ]; then  
    echo "Build is on Travis" 

    COMPONENT_VERSION=$(cat COMPONENT_VERSION 2> /dev/null)
    BUILD_IMAGE=${IMAGE_NAME}:${COMPONENT_VERSION}${COMPONENT_TAG_EXTENSION}

    echo "BUILD_IMAGE tag $BUILD_IMAGE"

    echo "modify deployment to point to the PR image"
    sed -i -e "s|image: .*:latest$|image: $BUILD_IMAGE|" deploy/standalone/operator.yaml

    #kind get kubeconfig > kindconfig
    sleep 15
fi

if ! kind get clusters &> /dev/null; then
    echo "create kind cluster"
    # Create a new Kubernetes cluster using KinD
    kind create cluster
    if [ $? != 0 ]; then
            exit $?;
    fi
fi

echo "path for container in YAML $(grep 'image: .*' deploy/standalone/operator.yaml)"

echo "load build image ($BUILD_IMAGE)to kind cluster"
kind load docker-image $BUILD_IMAGE
if [ $? != 0 ]; then
    exit $?;
fi

echo "switch kubeconfig to kind cluster"
kubectl cluster-info --context kind-kind

echo "applying channel operator to kind cluster"
kubectl apply -f deploy/standalone
if [ $? != 0 ]; then
    exit $?;
fi

echo "apply dependent CRDs"
kubectl apply -f deploy/dependent-crds

echo "apply channel CRDs"
kubectl apply -f deploy/crds

sleep 10
echo "check if channel deploy is created" 
kubectl get deploy multicluster-operators-channel
kubectl get po -A

if [ $? != 0 ]; then
    exit $?;
fi

exit 0;
