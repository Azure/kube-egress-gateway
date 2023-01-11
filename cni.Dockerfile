# syntax=docker/dockerfile:1
FROM alpine
COPY --from=baseimg /kube* /
ENTRYPOINT cp /kube* /opt/cni/bin/
