FROM registry.ci.openshift.org/stolostron/builder:go1.23-linux AS builder

WORKDIR /go/src/open-cluster-management.io/multicloud-operators-channel
COPY . .
RUN make -f Makefile build

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

RUN microdnf update -y && \
     microdnf clean all

ENV OPERATOR=/usr/local/bin/multicluster-operators-channel \
    USER_UID=1001 \
    USER_NAME=multicluster-operators-channel

# install operator binary
COPY --from=builder go/src/open-cluster-management.io/multicloud-operators-channel/build/_output/bin/multicluster-operators-channel ${OPERATOR}

COPY build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
