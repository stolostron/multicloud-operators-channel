# Copyright 2021 The Kubernetes Authors.
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

LOCAL_OS := $(shell uname)
ifeq ($(LOCAL_OS),Linux)
    TARGET_OS ?= linux
    XARGS_FLAGS="-r"
else ifeq ($(LOCAL_OS),Darwin)
    TARGET_OS ?= darwin
    XARGS_FLAGS=
else
    $(error "This system's OS $(LOCAL_OS) isn't recognized/supported")
endif

FINDFILES=find . \( -path ./.git -o -path ./.github \) -prune -o -type f
XARGS = xargs -0 ${XARGS_FLAGS}
CLEANXARGS = xargs ${XARGS_FLAGS}

REGISTRY = quay.io/open-cluster-management
VERSION = latest
IMAGE_NAME_AND_VERSION ?= $(REGISTRY)/multicloud-operators-channel:$(VERSION)
export GOPACKAGES   = $(shell go list ./... | grep -v /manager | grep -v /bindata  | grep -v /vendor | grep -v /internal | grep -v /build | grep -v /test | grep -v /e2e )

TEST_TMP :=/tmp
export KUBEBUILDER_ASSETS ?=$(TEST_TMP)/kubebuilder/bin
# Use setup-envtest to download envtest binaries from the new location (controller-tools releases).
# See: https://github.com/kubernetes-sigs/kubebuilder/discussions/4082
ENVTEST_K8S_VERSION ?= 1.30.0
ENVTEST_VERSION ?= v0.20.4

.PHONY: local

local:
	@GOOS=darwin common/scripts/gobuild.sh build/_output/bin/multicluster-operators-channel ./cmd/manager

.PHONY: build

build:
	@common/scripts/gobuild.sh build/_output/bin/multicluster-operators-channel ./cmd/manager

.PHONY: build-images

build-images:
	@docker build -t ${IMAGE_NAME_AND_VERSION} -f build/Dockerfile .

# build local linux/amd64 images on non-amd64 hosts such as Apple M3
build-images-non-amd64:
	@if docker buildx ls | grep -q "local-builder"; then \
		echo "Removing existing local-builder..."; \
		docker buildx rm local-builder; \
	fi

	docker buildx create --name local-builder --use
	docker buildx inspect local-builder --bootstrap
	docker buildx build --platform linux/amd64 -t ${IMAGE_NAME_AND_VERSION} -f build/Dockerfile --load .
	docker buildx rm local-builder

.PHONY: lint

lint: lint-all

.PHONY: lint-all

lint-all:lint-go

.PHONY: lint-go

lint-go:
	@${FINDFILES} -name '*.go' \( ! \( -name '*.gen.go' -o -name '*.pb.go' \) \) -print0 | ${XARGS} common/scripts/lint_go.sh

.PHONY: test

# Install setup-envtest (used to download envtest binaries from controller-tools GitHub releases).
ensure-kubebuilder-tools:
	@which setup-envtest >/dev/null 2>&1 || go install sigs.k8s.io/controller-runtime/tools/setup-envtest@$(ENVTEST_VERSION)

.PHONY: ensure-kubebuilder-tools

test: ensure-kubebuilder-tools
	KUBEBUILDER_ASSETS=$$(setup-envtest use -i -p path $(ENVTEST_K8S_VERSION)) go test -timeout 300s -v ./pkg/... -coverprofile=coverage.out
