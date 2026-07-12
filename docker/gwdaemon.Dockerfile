# syntax=docker/dockerfile:1
FROM registry.k8s.io/build-image/distroless-iptables:v0.9.5@sha256:291ebce2a508dfce8145399edc8f93daceda345ba81c3803202a1373d67a698c
USER 0:0
ARG MAIN_ENTRY
COPY --from=baseimg /${MAIN_ENTRY} /${MAIN_ENTRY}
ENTRYPOINT ["/${MAIN_ENTRY}"]
