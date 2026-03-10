# Stage 1: Build sandbox server
FROM golang:1.23-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/sandbox/ cmd/sandbox/
RUN CGO_ENABLED=0 go build -o /sandbox ./cmd/sandbox/

# Stage 2: Runtime with toolchains
FROM ubuntu:24.04

# Avoid interactive prompts during package installation.
ENV DEBIAN_FRONTEND=noninteractive

# System packages + Java
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential git curl wget jq ca-certificates \
    openjdk-21-jdk-headless maven \
    python3 python3-pip python3-venv \
    && rm -rf /var/lib/apt/lists/*

# Go 1.23
RUN curl -fsSL https://go.dev/dl/go1.23.6.linux-amd64.tar.gz | tar -C /usr/local -xz
ENV PATH="/usr/local/go/bin:/go/bin:${PATH}" \
    GOPATH=/go

# Node.js 22 LTS
RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y nodejs \
    && npm install -g typescript vitest jest \
    && rm -rf /var/lib/apt/lists/*

# Create sandbox user and workspace directory.
RUN useradd -m -s /bin/bash -u 1000 sandbox \
    && mkdir -p /workspace /go/pkg/mod \
    && chown -R sandbox:sandbox /workspace /go

COPY --from=builder /sandbox /usr/local/bin/sandbox

USER sandbox
WORKDIR /workspace
EXPOSE 8090

ENTRYPOINT ["sandbox"]
CMD ["--addr", ":8090", "--workspace", "/workspace"]
