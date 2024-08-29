# syntax=docker/dockerfile:1
FROM gcr.io/distroless/static:latest
ARG MAIN_ENTRY
COPY --from=baseimg /${MAIN_ENTRY} /
ENTRYPOINT ["/${MAIN_ENTRY}"]
