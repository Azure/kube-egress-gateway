# syntax=docker/dockerfile:1
FROM registry.k8s.io/build-image/distroless-iptables:v0.9.0@sha256:066def23165ef4bb68f6cd375283ad99442c434c7a2e3d59b7ff1f2a0f90c0bf
USER 0:0
ARG MAIN_ENTRY
COPY --from=baseimg /${MAIN_ENTRY} /${MAIN_ENTRY}
ENTRYPOINT ["/${MAIN_ENTRY}"]
