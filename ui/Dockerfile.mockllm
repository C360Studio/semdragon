# Mock LLM server Dockerfile — OpenAI-compatible stub for E2E testing.
FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /mockllm ./cmd/mockllm

FROM alpine:3.21
RUN apk --no-cache add wget
COPY --from=builder /mockllm /mockllm
EXPOSE 9090
HEALTHCHECK --interval=5s --timeout=3s CMD wget -q --spider http://localhost:9090/health
CMD ["/mockllm"]
