# syntax=docker/dockerfile:1
FROM gcr.io/distroless/static:latest@sha256:3f2b64ef97bd285e36132c684e6b2ae8f2723293d09aae046196cca64251acac
ARG MAIN_ENTRY
COPY --from=baseimg /${MAIN_ENTRY} /
ENTRYPOINT ["/${MAIN_ENTRY}"]
