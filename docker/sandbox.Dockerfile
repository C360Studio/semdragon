# Stage 1: Build sandbox server
FROM golang:1.25-alpine AS builder
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
    build-essential git curl wget jq unzip ca-certificates \
    openjdk-21-jdk-headless maven \
    python3 python3-pip python3-venv \
    && rm -rf /var/lib/apt/lists/*

# Go 1.25
RUN ARCH=$(dpkg --print-architecture) && \
    curl -fsSL "https://go.dev/dl/go1.25.3.linux-${ARCH}.tar.gz" | tar -C /usr/local -xz
ENV PATH="/usr/local/go/bin:/go/bin:${PATH}" \
    GOPATH=/go

# Gradle 8.14
RUN curl -fsSL https://services.gradle.org/distributions/gradle-8.14-bin.zip -o /tmp/gradle.zip \
    && unzip -q /tmp/gradle.zip -d /opt \
    && rm /tmp/gradle.zip \
    && ln -s /opt/gradle-8.14/bin/gradle /usr/local/bin/gradle
ENV GRADLE_HOME=/opt/gradle-8.14

# Node.js 22 LTS
RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y nodejs \
    && npm install -g typescript vitest jest \
    && rm -rf /var/lib/apt/lists/*

# Create sandbox user. Sandbox is the sole writer of /workspace and /repos.
RUN useradd -m -s /bin/bash -U sandbox \
    && mkdir -p /workspace /repos /go/pkg/mod \
    && chown -R sandbox:sandbox /workspace /repos \
    && chown -R sandbox:sandbox /go

COPY --from=builder /sandbox /usr/local/bin/sandbox

USER sandbox
WORKDIR /workspace
EXPOSE 8090

ENTRYPOINT ["sandbox"]
CMD ["--addr", ":8090", "--workspace", "/workspace", "--repos-dir", "/repos"]
