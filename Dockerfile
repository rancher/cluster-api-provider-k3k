FROM golang:1.24 AS builder

ARG TARGETOS
ARG TARGETARCH
ARG K3K_VERSION

WORKDIR /workspace

# Download and unzip the upstream K3K chart.
RUN wget https://github.com/rancher/k3k/releases/download/chart-${K3K_VERSION}/k3k-${K3K_VERSION}.tgz && \
    gunzip k3k-${K3K_VERSION}.tgz && \
    tar xvf k3k-${K3K_VERSION}.tar && \
    mkdir -p charts && \
    mv k3k charts/k3k

COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

# Copy the go source
COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY internal internal

# Build
# the GOARCH has not a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o manager cmd/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .
COPY --from=builder /workspace/charts /charts
USER 65532:65532
ARG K3K_VERSION
ENV K3K_VERSION=${K3K_VERSION}
ENTRYPOINT ["/manager"]
