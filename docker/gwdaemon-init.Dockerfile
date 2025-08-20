# syntax=docker/dockerfile:1
FROM gcr.io/distroless/static:latest@sha256:3d0f463de06b7ddff27684ec3bfd0b54a425149d0f8685308b1fdf297b0265e9
WORKDIR /workspace
COPY --from=tool /add-netns .
ENTRYPOINT ["./add-netns"]
