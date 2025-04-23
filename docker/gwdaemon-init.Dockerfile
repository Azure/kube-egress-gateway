# syntax=docker/dockerfile:1
FROM mcr.microsoft.com/cbl-mariner/base/core:2.0@sha256:8deab931d4af66264253cce66471a04742dc6f8d5d175d4c18e930eda615c0aa
RUN yum install -y iproute
ENTRYPOINT ["/bin/sh", "-c", "ip netns exec ns-static-egress-gateway ip a || ip netns add ns-static-egress-gateway"]

