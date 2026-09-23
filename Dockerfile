FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG PASS=testpass123
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X main.localPassword=${PASS}" \
    -o /tether .

FROM alpine:3.21
RUN apk add --no-cache bash openssh-client
COPY --from=builder /tether /tether
ENTRYPOINT ["/tether"]
