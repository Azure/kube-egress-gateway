# syntax=docker/dockerfile:1
FROM registry.k8s.io/build-image/distroless-iptables:v0.8.0@sha256:3c55589bb92403afd8926d8eb677eaa20ea7a380594db994cf7e27d4fdf53f78
USER 0:0
ARG MAIN_ENTRY
COPY --from=baseimg /${MAIN_ENTRY} /${MAIN_ENTRY}
ENTRYPOINT ["/${MAIN_ENTRY}"]
