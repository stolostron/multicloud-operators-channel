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


# need to find a way to use the Makefile to set these
REGISTRY=quay.io/open-cluster-management

if [ "$TRAVIS_BUILD" == 1 ]; then  
    echo "Build is on local"  

    IMG=$(cat ../COMPONENT_NAME 2> /dev/null)
    COMPONENT_VERSION=$(cat ../COMPONENT_VERSION 2> /dev/null)
    IMAGE_NAME_AND_VERSION=${REGISTRY}/${IMG}
    BUILD_IMAGE=${IMAGE_NAME_AND_VERSION}:${COMPONENT_VERSION}${COMPONENT_TAG_EXTENSION}

    echo "BUILD_IMAGE tag $BUILD_IMAGE"
    sed "s|image: .*:latest$|image: $BUILD_IMAGE|" ../deploy/standalone/operator.yaml
else
    echo "Build is on Travis" 

    echo "get kubectl binary"
    # Download and install kubectl
    curl -LO https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl && chmod +x kubectl && sudo mv kubectl /usr/local/bin/
    if [ $? != 0 ]; then
        exit $?;
    fi

    # Download and install KinD
    GO111MODULE=on go get sigs.k8s.io/kind


    IMG=$(cat COMPONENT_NAME 2> /dev/null)
    COMPONENT_VERSION=$(cat COMPONENT_VERSION 2> /dev/null)
    IMAGE_NAME_AND_VERSION=${REGISTRY}/${IMG}
    BUILD_IMAGE=${IMAGE_NAME_AND_VERSION}:${COMPONENT_VERSION}${COMPONENT_TAG_EXTENSION}

    echo "BUILD_IMAGE tag $BUILD_IMAGE"

    echo "modify deployment to point to the PR image"
    sed -i -e "s|image: .*:latest$|image: $BUILD_IMAGE|" deploy/standalone/operator.yaml
    grep 'image: .*' deploy/standalone/operator.yaml

    echo "create kind cluster"
    # Create a new Kubernetes cluster using KinD
    kind create cluster
    if [ $? != 0 ]; then
        exit $?;
    fi

    sleep 30
fi

kind get kubeconfig > kindconfig

echo "load build image to kind cluster"
kind load docker-image $BUILD_IMAGE
if [ $? != 0 ]; then
    exit $?;
fi

kubectl cluster-info --context kind-kind


echo "applying channel operator to kind cluster"
kubectl apply -f deploy/standalone
if [ $? != 0 ]; then
    exit $?;
fi

sleep 30
kubectl get po -A

echo "check if channel deploy is created" && kubectl get deploy multicluster-operators-channel

if [ $? != 0 ]; then
    exit $?;
fi

exit 0;
