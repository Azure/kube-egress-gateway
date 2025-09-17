# syntax=docker/dockerfile:1
FROM registry.k8s.io/build-image/distroless-iptables:v0.8.2@sha256:89d2f2fc47a1efc66b017c131bffb9e4837d64821044475cbfea91a609b8f93d
USER 0:0
ARG MAIN_ENTRY
COPY --from=baseimg /${MAIN_ENTRY} /${MAIN_ENTRY}
ENTRYPOINT ["/${MAIN_ENTRY}"]
