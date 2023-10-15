# syntax=docker/dockerfile:1
FROM registry.k8s.io/build-image/distroless-iptables:v0.4.1
USER 0:0
COPY --from=baseimg /${MAIN_ENTRY} .
ENTRYPOINT [${MAIN_ENTRY}]
