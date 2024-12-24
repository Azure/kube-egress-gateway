# syntax=docker/dockerfile:1
FROM registry.k8s.io/build-image/distroless-iptables:v0.6.6@sha256:be1e8d4d451ada7e39ba4e44fbd8f58ba50feb4f4b1906df7b2bbb424efb06f3
USER 0:0
ARG MAIN_ENTRY
COPY --from=baseimg /${MAIN_ENTRY} /${MAIN_ENTRY}
ENTRYPOINT ["/${MAIN_ENTRY}"]
