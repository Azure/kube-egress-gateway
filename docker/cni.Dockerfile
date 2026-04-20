# syntax=docker/dockerfile:1
FROM gcr.io/distroless/static:latest@sha256:47b2d72ff90843eb8a768b5c2f89b40741843b639d065b9b937b07cd59b479c6
WORKDIR /workspace
COPY --from=baseimg /kube-egress-cni .
COPY --from=tool /copy .
ENTRYPOINT ["./copy", "-s", "./kube-egress-cni", "-d", "/opt/cni/bin/"]
