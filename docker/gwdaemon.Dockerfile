# syntax=docker/dockerfile:1
FROM registry.k8s.io/build-image/distroless-iptables:v0.9.1@sha256:d0ae8abdbeb270570664a7364ded872c5e8f58eea09d8fb4b46e972b7291eacb
USER 0:0
ARG MAIN_ENTRY
COPY --from=baseimg /${MAIN_ENTRY} /${MAIN_ENTRY}
ENTRYPOINT ["/${MAIN_ENTRY}"]
