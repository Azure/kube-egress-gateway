# syntax=docker/dockerfile:1
FROM registry.k8s.io/build-image/distroless-iptables:v0.8.7@sha256:cdac80d8683f9d8d3aa177383e3a5ab166df91d4e884199023d4a5b088f6015f
USER 0:0
ARG MAIN_ENTRY
COPY --from=baseimg /${MAIN_ENTRY} /${MAIN_ENTRY}
ENTRYPOINT ["/${MAIN_ENTRY}"]
