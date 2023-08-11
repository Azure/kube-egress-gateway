# syntax=docker/dockerfile:1
FROM registry.k8s.io/build-image/distroless-iptables:v0.2.7
USER 0:0
COPY --from=baseimg /${MAIN_ENTRY} .
ENTRYPOINT [${MAIN_ENTRY}]
