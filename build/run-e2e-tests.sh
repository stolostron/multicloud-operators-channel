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
# See the License for the specific language governing permissions and# limitations under the License.


###!!!!!!!! On travis this script is run on the .git level
echo -e "E2E TESTS GO HERE!"

# need to find a way to use the Makefile to set these
REGISTRY=quay.io/open-cluster-management
IMG=$(cat COMPONENT_NAME 2> /dev/null)
IMAGE_NAME=${REGISTRY}/${IMG}
BUILD_IMAGE=${IMAGE_NAME}:latest

TEST_API_SERVER_IMG="quay.io/open-cluster-management/applifecycle-backend-e2e:latest"

if [ "$TRAVIS_BUILD" != 1 ]; then
    echo -e "Build is on Travis" 

    echo -e "\nDownload and install KinD\n"
    GO111MODULE=on go get sigs.k8s.io/kind

    echo -e "\nGet kubectl binary\n"
    # Download and install kubectl
    curl -LO https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl && chmod +x kubectl && sudo mv kubectl /usr/local/bin/

    COMPONENT_VERSION=$(cat COMPONENT_VERSION 2> /dev/null)
    BUILD_IMAGE=${IMAGE_NAME}:${COMPONENT_VERSION}${COMPONENT_TAG_EXTENSION}

    echo -e "\nBUILD_IMAGE tag $BUILD_IMAGE\n"
    echo -e "Modify deployment to point to the PR image\n"
    sed -i -e "s|image: .*:latest$|image: $BUILD_IMAGE|" deploy/standalone/operator.yaml

    TEST_API_SERVER_IMG="quay.io/open-cluster-management/applifecycle-backend-e2e:0.0.1"

    echo -e "\nPull API test server image\n"
    docker pull $TEST_API_SERVER_IMG

    kind create cluster
    if [ $? != 0 ]; then
            exit $?;
    fi
    sleep 15

else
    echo -e "\nBuild is on Local ENV, will delete the API container first\n"
    docker kill e2e
fi

echo -e "\nPath for container in YAML $(grep 'image: .*' deploy/standalone/operator.yaml)\n"

echo -e "\nLoad build image ($BUILD_IMAGE)to kind cluster\n"
kind load docker-image $BUILD_IMAGE
if [ $? != 0 ]; then
    exit $?;
fi

echo -e "\nSwitch kubeconfig to kind cluster\n"
kubectl cluster-info --context kind-kind

echo -e "\nApplying channel operator to kind cluster\n"
kubectl apply -f deploy/standalone
if [ $? != 0 ]; then
    exit $?;
fi

echo -e "\nApply dependent CRDs\n"
kubectl apply -f deploy/dependent-crds

echo -e "\nApply channel CRDs\n"
kubectl apply -f deploy/crds

if [ "$TRAVIS_BUILD" != 1 ]; then
    echo -e "\nwait for pod to be ready\n"
    sleep 40
fi

echo -e "\nCheck if channel deploy is created\n" 
kubectl get deploy multicluster-operators-channel
kubectl get po -A

if [ $? != 0 ]; then
    exit $?;
fi


echo -e "\ndelete the running container: ${CONTAINER_NAME} if exist"
docker rm -f ${CONTAINER_NAME} || true

echo -e "\nRun API test server\n"
mkdir -p cluster_config
kind get kubeconfig > cluster_config/hub

docker run -v $PWD/cluster_config:/go/configs -p 8765:8765 --env CONFIGS="/go/configs" --name e2e -d --rm $TEST_API_SERVER_IMG --v 0

docker logs ${CONTAINER_NAME}
docker ps
sleep 10
curl http://localhost:8765/cluster
