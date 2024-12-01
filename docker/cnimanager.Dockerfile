# syntax=docker/dockerfile:1
FROM gcr.io/distroless/static:latest@sha256:5c7e2b465ac6a2a4e5f4f7f722ce43b147dabe87cb21ac6c4007ae5178a1fa58
ARG MAIN_ENTRY
COPY --from=baseimg /${MAIN_ENTRY} /
ENTRYPOINT ["/${MAIN_ENTRY}"]
