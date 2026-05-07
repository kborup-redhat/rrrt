FROM registry.redhat.io/ubi9/go-toolset:1.22 AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION}" -o /tmp/rrrt-analyzer ./cmd/analyzer/

FROM registry.redhat.io/ubi9/ubi-minimal:latest

COPY --from=builder /tmp/rrrt-analyzer /usr/local/bin/rrrt-analyzer

RUN mkdir -p /output

USER 1001

ENTRYPOINT ["rrrt-analyzer"]
