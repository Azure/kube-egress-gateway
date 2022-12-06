# syntax=docker/dockerfile:1
ARG BASE_IMAGE=gcr.io/distroless/static
# Build the manager binary
FROM --platform=$BUILDPLATFORM golang:1.19 as builder 
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

ARG MAIN_ENTRY=kube-egress-gateway-controller
ARG TARGETARCH
FROM builder as base
WORKDIR /workspace
# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -a -o ${MAIN_ENTRY}  ./cmd/${MAIN_ENTRY}/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
# FROM gcr.io/distroless/static
FROM $BASE_IMAGE
WORKDIR /
COPY --from=base /workspace/${MAIN_ENTRY} .
USER 65532:65532

ENTRYPOINT [${MAIN_ENTRY}]


