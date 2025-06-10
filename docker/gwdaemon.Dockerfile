# syntax=docker/dockerfile:1
FROM registry.k8s.io/build-image/distroless-iptables:v0.7.6@sha256:e40791e0d57a3d428893a28c0bab123bdaf99a026738785bc00d8dceb94cde65
USER 0:0
ARG MAIN_ENTRY
COPY --from=baseimg /${MAIN_ENTRY} /${MAIN_ENTRY}
ENTRYPOINT ["/${MAIN_ENTRY}"]
