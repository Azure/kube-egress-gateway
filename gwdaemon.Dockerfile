# syntax=docker/dockerfile:1
FROM baseosscr.azurecr.io/build-image/distroless-iptables-amd64:v0.1.2
USER 0:0
COPY --from=baseimg /${MAIN_ENTRY} .
ENTRYPOINT [${MAIN_ENTRY}]
