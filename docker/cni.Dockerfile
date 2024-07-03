# syntax=docker/dockerfile:1
FROM mcr.microsoft.com/mirror/docker/library/alpine:3.16
COPY --from=baseimg /kube* /
USER 0:0
ENTRYPOINT cp /kube* /opt/cni/bin/
