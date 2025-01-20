# syntax=docker/dockerfile:1
FROM registry.k8s.io/build-image/distroless-iptables:v0.7.0@sha256:f65e3f1d8a65bfd45cb5eaa60a3d4ed67c7c37a09ca2806bbe1696096b6f1945
USER 0:0
ARG MAIN_ENTRY
COPY --from=baseimg /${MAIN_ENTRY} /${MAIN_ENTRY}
ENTRYPOINT ["/${MAIN_ENTRY}"]
