# syntax=docker/dockerfile:1
# Build the manager binary
FROM --platform=$BUILDPLATFORM mcr.microsoft.com/oss/go/microsoft/golang:1.26.3@sha256:286560499f88a68ff68ce11520271185294251e37411a0881c3777d402e8d017 AS builder 
WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download
# Copy the go source
COPY cmd/ cmd/
COPY api/ api/
COPY controllers/ controllers/
COPY pkg/ pkg/

ARG MAIN_ENTRY
ARG TARGETARCH
FROM builder AS base
WORKDIR /workspace
# Build
RUN GOEXPERIMENT=nosystemcrypto CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -a -o ${MAIN_ENTRY}  ./cmd/${MAIN_ENTRY}/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot@sha256:e3f945647ffb95b5839c07038d64f9811adf17308b9121d8a2b87b6a22a80a39
ARG MAIN_ENTRY
WORKDIR /
COPY --from=base /workspace/${MAIN_ENTRY} .
USER 65532:65532

ENTRYPOINT ["${MAIN_ENTRY}"]


