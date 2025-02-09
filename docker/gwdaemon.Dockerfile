# syntax=docker/dockerfile:1
FROM registry.k8s.io/build-image/distroless-iptables:v0.7.1@sha256:471f24e3cee7a253fb0803c66c38398f99e9557f6ad74ee8e52b924805714994
USER 0:0
ARG MAIN_ENTRY
COPY --from=baseimg /${MAIN_ENTRY} /${MAIN_ENTRY}
ENTRYPOINT ["/${MAIN_ENTRY}"]
