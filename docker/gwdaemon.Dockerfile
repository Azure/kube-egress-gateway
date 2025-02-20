# syntax=docker/dockerfile:1
FROM registry.k8s.io/build-image/distroless-iptables:v0.7.3@sha256:25cc119a89bf860f964e5c47d0422eabcd260424692a2b6d4e7a74d5c3bb2231
USER 0:0
ARG MAIN_ENTRY
COPY --from=baseimg /${MAIN_ENTRY} /${MAIN_ENTRY}
ENTRYPOINT ["/${MAIN_ENTRY}"]
