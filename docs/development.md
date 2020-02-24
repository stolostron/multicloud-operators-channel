# Development Guide

- build your own binary or image
You can clone this repo and modify it, then you can call the `make build` to build your own binary file.

In order to run the built binary, you can do `go run cmd/manager/main.go <flags>`. You can run `go run cmd/manager/main.go -h` to list and understand the flags.

In addition, you can call `make build-image` to build a docker image, then you can run similar `go` command with docker.

- test your code

We have a test target at the `Makefile`, which is calling the `go test -v ./...` to run all the tests inside your project. If you want to run specific test, you can call `go test --run <test> -v`.

