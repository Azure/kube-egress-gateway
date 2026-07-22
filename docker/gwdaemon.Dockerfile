# syntax=docker/dockerfile:1
FROM registry.k8s.io/build-image/distroless-iptables:v0.9.6@sha256:33f27a340efd1f0a0341bd85755f1752af00b27debaf2f2876a3ef5dcc07f84a
USER 0:0
ARG MAIN_ENTRY
COPY --from=baseimg /${MAIN_ENTRY} /${MAIN_ENTRY}
ENTRYPOINT ["/${MAIN_ENTRY}"]
