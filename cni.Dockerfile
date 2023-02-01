# syntax=docker/dockerfile:1
FROM alpine
COPY --from=baseimg /kube* /
USER 0:0
ENTRYPOINT cp /kube* /opt/cni/bin/
