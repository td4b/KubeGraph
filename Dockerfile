# ---- Build kubegraph binary ----
FROM golang:1.22-alpine AS builder

# Optional: install git if you need modules
RUN apk add --no-cache git

WORKDIR /app

# Copy your Go source
COPY ../main.go main.go
COPY ../go.mod go.mod
COPY ../go.sum go.sum

# Build the binary
RUN go mod tidy && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/kubegraph .

# ---- Final image ----
FROM alpine:3.19

# Install wget, curl, tar, sha256sum, kustomize dependencies
RUN apk add --no-cache wget curl tar bash coreutils

# Download kustomize
RUN wget -q -O /usr/local/bin/kustomize \
  https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize/v5.0.3/kustomize_v5.0.3_linux_amd64 && \
  chmod +x /usr/local/bin/kustomize

# Copy the built kubegraph binary from builder stage
COPY --from=builder /out/kubegraph /usr/local/bin/kubegraph

# Set entrypoint to CMP server
ENTRYPOINT ["/var/run/argocd/argocd-cmp-server"]
