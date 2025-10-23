# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

This is the `multicloud-operators-channel` repository, a Kubernetes operator that manages Channels for Open Cluster Management. Channels define source repositories (Git, Helm, Object Storage) that clusters can subscribe to via subscriptions. This is part of the broader [multicloud-operators-subscription](https://github.com/stolostron/multicloud-operators-subscription) ecosystem.

## Development Commands

### Building
- `make build` - Build the binary to `build/_output/bin/multicluster-operators-channel`
- `make local` - Build for macOS/Darwin specifically
- `make build-images` - Build Docker image
- `make build-images-non-amd64` - Build for Linux/amd64 on non-amd64 hosts (e.g., Apple M3)

### Testing
- `make test` - Run all tests with coverage (automatically downloads kubebuilder tools)
- `go test --run <test> -v` - Run specific test
- Tests require kubebuilder tools which are automatically downloaded to `/tmp/kubebuilder/bin`

### Linting
- `make lint` or `make lint-all` - Run all linting
- `make lint-go` - Run Go-specific linting using `common/scripts/lint_go.sh`

### Running Locally
- `go run cmd/manager/main.go <flags>` - Run the manager directly
- `go run cmd/manager/main.go -h` - See available flags

### Pre-commit Requirements
Per CONTRIBUTING.md, before submitting PRs run:
```shell
make build
make test
```

## Architecture

### Core Components

- **APIs** (`pkg/apis/apps/v1/`): Defines the Channel CRD with support for multiple channel types:
  - `namespace` - Namespace-based channels
  - `helmrepo` - Helm repository channels
  - `objectbucket` - Object storage channels (S3-compatible)
  - `github`/`git` - Git repository channels

- **Controller** (`pkg/controller/`): Kubernetes controller logic for managing Channel resources

- **Webhook** (`pkg/webhook/`): Admission webhook for validating Channel resources, includes certificate management

- **Utils** (`pkg/utils/`): Shared utilities for Git, Helm, S3, and other operations

### Entry Point
- `cmd/manager/main.go` - Main entry point that sets up flags and starts the manager
- `cmd/manager/exec/` - Contains manager execution logic and options

### Channel Types and Features

The Channel CRD supports:
- Multiple repository types (Git, Helm, Object Storage)
- Authentication via `secretRef` (credentials for Git/Helm, AWS creds for S3)
- Configuration via `configMapRef` (e.g., `insecureSkipVerify` for HTTPS)
- Channel gates for promoting Deployables with label selectors
- Source namespace filtering

### Key Dependencies

- Built with Go 1.23+ using controller-runtime framework
- Kubernetes APIs (v0.29.1)
- Helm SDK (v3.14.4) for Helm repository support
- AWS SDK v2 for S3/Object Storage support
- Open Cluster Management APIs (v0.13.0)

## Project Structure

- `cmd/` - Main application entry points
- `pkg/apis/` - API definitions and CRD types
- `pkg/controller/` - Controller implementations
- `pkg/webhook/` - Admission webhook logic
- `pkg/utils/` - Shared utilities
- `deploy/` - Kubernetes deployment manifests
- `build/` - Docker build files and scripts
- `common/scripts/` - Build and lint scripts
- `examples/` - Example Channel configurations
- `e2e/` - End-to-end tests

## Integration Context

This operator works in conjunction with:
- `multicloud-operators-subscription` - Consumes Channels to create Subscriptions
- Open Cluster Management hub - Provides multi-cluster distribution capabilities
- Source repositories - Git, Helm, and Object Storage backends that Channels reference