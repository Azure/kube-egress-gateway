# syntax=docker/dockerfile:1
FROM registry.k8s.io/build-image/distroless-iptables:v0.8.4@sha256:a59cba69fdb78c15903a561b6824b4ca5644adcc33d745530d168efa8b87160b
USER 0:0
ARG MAIN_ENTRY
COPY --from=baseimg /${MAIN_ENTRY} /${MAIN_ENTRY}
ENTRYPOINT ["/${MAIN_ENTRY}"]
