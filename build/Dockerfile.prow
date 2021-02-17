FROM registry.access.redhat.com/ubi8/ubi-minimal:latest

RUN microdnf update && \
     microdnf clean all

ENV OPERATOR=/usr/local/bin/multicluster-operators-channel \
    USER_UID=1001 \
    USER_NAME=multicluster-operators-channel

# install operator binary
COPY build/_output/bin/multicluster-operators-channel ${OPERATOR}

COPY build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
