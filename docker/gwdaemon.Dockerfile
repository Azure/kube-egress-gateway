# syntax=docker/dockerfile:1
FROM registry.k8s.io/build-image/distroless-iptables:v0.7.5@sha256:6a6f15570fe8d83afadbd8cdf92fd8b58602a5a2916ca8acb4314d4bee8d336c
USER 0:0
ARG MAIN_ENTRY
COPY --from=baseimg /${MAIN_ENTRY} /${MAIN_ENTRY}
ENTRYPOINT ["/${MAIN_ENTRY}"]
