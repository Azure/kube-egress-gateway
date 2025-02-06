# syntax=docker/dockerfile:1
# Build the manager binary
FROM --platform=$BUILDPLATFORM mcr.microsoft.com/oss/go/microsoft/golang:1.23.6@sha256:2d8c09c93157b49f2f8f0523d907620d022e6d354e670b60a83125067f0c8960 AS builder 
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
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -a -o ${MAIN_ENTRY}  ./cmd/${MAIN_ENTRY}/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot@sha256:6ec5aa99dc335666e79dc64e4a6c8b89c33a543a1967f20d360922a80dd21f02
ARG MAIN_ENTRY
WORKDIR /
COPY --from=base /workspace/${MAIN_ENTRY} .
USER 65532:65532

ENTRYPOINT ["${MAIN_ENTRY}"]


