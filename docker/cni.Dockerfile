# syntax=docker/dockerfile:1
FROM gcr.io/distroless/static:latest@sha256:95ea148e8e9edd11cc7f639dc11825f38af86a14e5c7361753c741ceadef2167
USER 0:0
WORKDIR /workspace
COPY --from=baseimg /kube-egress-cni .
COPY --from=tool /copy .
ENTRYPOINT ["./copy", "-s", "./kube-egress-cni", "-d", "/opt/cni/bin/"]
