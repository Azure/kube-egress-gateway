# syntax=docker/dockerfile:1
FROM registry.k8s.io/build-image/distroless-iptables:v0.6.1
USER 0:0
ARG MAIN_ENTRY
COPY --from=baseimg /${MAIN_ENTRY} /${MAIN_ENTRY}
ENTRYPOINT [/${MAIN_ENTRY}]
