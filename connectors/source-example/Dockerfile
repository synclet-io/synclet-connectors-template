FROM golang:1.25-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -trimpath -ldflags "-s -w" -o /connector .

FROM alpine:3.24
RUN apk --no-cache add ca-certificates

COPY --from=builder /connector /connector

ENV AIRBYTE_ENTRYPOINT=/connector
ENTRYPOINT ["/connector"]
