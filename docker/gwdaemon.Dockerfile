# syntax=docker/dockerfile:1
FROM registry.k8s.io/build-image/distroless-iptables:v0.6.4@sha256:86dfa6ce123df424af562a6b66404e6f758fbf6d8a5fd96ccc5812209052e25f
USER 0:0
ARG MAIN_ENTRY
COPY --from=baseimg /${MAIN_ENTRY} /${MAIN_ENTRY}
ENTRYPOINT ["/${MAIN_ENTRY}"]
