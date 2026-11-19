# syntax=docker/dockerfile:1
FROM registry.k8s.io/build-image/distroless-iptables:v0.11.1@sha256:867d39fa67cf859730e6287b565447be8b41c9885dea13c686bd63f5df643a40
USER 0:0
ARG MAIN_ENTRY
COPY --from=baseimg /${MAIN_ENTRY} /${MAIN_ENTRY}
ENTRYPOINT ["/${MAIN_ENTRY}"]
