# syntax=docker/dockerfile:1
FROM registry.k8s.io/build-image/distroless-iptables:v0.9.3@sha256:2ca39206f848af736c1602f33f01b50da79bf8f2aff9348846b5c9af61afebfa
USER 0:0
ARG MAIN_ENTRY
COPY --from=baseimg /${MAIN_ENTRY} /${MAIN_ENTRY}
ENTRYPOINT ["/${MAIN_ENTRY}"]
