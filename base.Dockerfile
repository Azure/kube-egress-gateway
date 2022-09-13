# Build the manager binary
FROM --platform=$BUILDPLATFORM golang:1.19 as builder 
WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

ARG MAIN_ENTRY=kube-egress-gateway-controller
ARG TARGETARCH
FROM builder as base
WORKDIR /workspace
# Copy the go source
COPY cmd/ cmd/
COPY api/ api/
COPY controllers/ controllers/
# https://github.com/kubernetes-sigs/kubebuilder-declarative-pattern/blob/master/docs/addon/walkthrough/README.md#adding-a-manifest
# Stage channels and make readable
COPY channels/ /channels/
RUN chmod -R a+rx /channels/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -a -o ${MAIN_ENTRY}  ./cmd/${MAIN_ENTRY}/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=base /workspace/${MAIN_ENTRY} .
# copy channels
COPY --from=base /channels /channels

USER 65532:65532

ENTRYPOINT [${MAIN_ENTRY}]


