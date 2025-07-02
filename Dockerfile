# ---- Build kubegraph binary ----
FROM golang:1.24.4-alpine AS builder

# Optional: install git if you need modules
RUN apk add --no-cache git

WORKDIR /app

# Copy your Go source
COPY . .

# Build the binary
RUN go mod tidy && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/kubegraph ./cmd/kubegraph

# ---- Final image ----
FROM alpine:3.19

# Install wget, curl, tar, sha256sum, kustomize dependencies
RUN apk add --no-cache wget curl tar bash coreutils

# Download kustomize
RUN wget -q https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2Fv5.7.0/kustomize_v5.7.0_linux_amd64.tar.gz && \
    tar -xzf kustomize_v5.7.0_linux_amd64.tar.gz && \
    mv kustomize /usr/local/bin/kustomize && \
    chmod +x /usr/local/bin/kustomize && \
    rm kustomize_v5.7.0_linux_amd64.tar.gz

# Copy the built kubegraph binary from builder stage
COPY --from=builder /out/kubegraph /usr/local/bin/kubegraph

# Set entrypoint to CMP server
ENTRYPOINT ["/var/run/argocd/argocd-cmp-server"]
