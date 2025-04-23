# syntax=docker/dockerfile:1
FROM mcr.microsoft.com/mirror/docker/library/alpine:3.16@sha256:452e7292acee0ee16c332324d7de05fa2c99f9994ecc9f0779c602916a672ae4
RUN apk add --no-cache iproute2
ENTRYPOINT ["/bin/sh", "-c", "ip netns exec ns-static-egress-gateway ip a || ip netns add ns-static-egress-gateway"]

