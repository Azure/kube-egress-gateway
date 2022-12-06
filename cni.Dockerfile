# syntax=docker/dockerfile:1
FROM alpine
COPY --from=baseimg /kube-egress-cni /
ENTRYPOINT cp /kube-egress-cni /opt/cni/bin/kube-egress-cni
