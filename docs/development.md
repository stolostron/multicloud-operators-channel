# Development Guide

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**  *generated with [DocToc](https://github.com/thlorenz/doctoc)*

- [Development Guide](#development-guide)
    - [Build a binary or image](#build-a-binary-or-image)
    - [Build a local image](#test-your-code)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Build a binary or image

You can clone this repo and modify it, then you can call the `make build` to build your own binary file.

In order to run the built binary, you can run `go run cmd/manager/main.go <flags>`. 

You can run `go run cmd/manager/main.go -h` to list and understand the flags.

In addition, you can call `make build-image` to build a Docker image, then you can run similar `go` command with docker.

## Test your code

We have a test target at the `Makefile`, which is calling the `go test -v ./...` to run all the tests inside your project. If you want to run specific test, you can call `go test --run <test> -v`.

