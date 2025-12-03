# syntax=docker/dockerfile:1
FROM registry.k8s.io/build-image/distroless-iptables:v0.8.5@sha256:52286f9b3d4a4d658083aac924447187881833298622d4a31bf9241bae7bac68
USER 0:0
ARG MAIN_ENTRY
COPY --from=baseimg /${MAIN_ENTRY} /${MAIN_ENTRY}
ENTRYPOINT ["/${MAIN_ENTRY}"]
