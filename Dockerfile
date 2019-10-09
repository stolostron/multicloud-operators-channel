FROM scratch

COPY build/_output/bin/multicloud-operators-channel .
CMD ["./multicloud-operators-channel"]
