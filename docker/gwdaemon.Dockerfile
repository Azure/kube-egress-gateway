# syntax=docker/dockerfile:1
FROM registry.k8s.io/build-image/distroless-iptables:v0.11.0@sha256:bedc7f0c0dc8e42a316f1cd6d6ee321eb4159e1f75ac30875bdfcf13446bc7a0
USER 0:0
ARG MAIN_ENTRY
COPY --from=baseimg /${MAIN_ENTRY} /${MAIN_ENTRY}
ENTRYPOINT ["/${MAIN_ENTRY}"]
