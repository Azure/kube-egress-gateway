# syntax=docker/dockerfile:1
FROM mcr.microsoft.com/aks/devinfra/base-os-runtime-nettools:master.221105.1
USER 0:0
COPY --from=baseimg /${MAIN_ENTRY} .
ENTRYPOINT [${MAIN_ENTRY}]
