# syntax=docker/dockerfile:1
FROM registry.k8s.io/build-image/distroless-iptables:v0.6.5@sha256:3fdb391f7e76cc2301dea2bbb7973b80a89c400dbd967ece59880d03b2b62e65
USER 0:0
ARG MAIN_ENTRY
COPY --from=baseimg /${MAIN_ENTRY} /${MAIN_ENTRY}
ENTRYPOINT ["/${MAIN_ENTRY}"]
