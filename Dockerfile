# syntax=docker/dockerfile:1
FROM golang:1.20-alpine AS build
RUN apk add gcc git linux-headers musl-dev
WORKDIR /msak

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY ./ ./
RUN ./build.sh

FROM alpine:3.17.3
WORKDIR /msak
COPY --from=build /msak/msak-server /msak/
COPY --from=build /msak/generate-schema /msak/

# Generate msak's JSON schemas.
RUN /msak/generate-schema -throughput1=/msak/throughput1.json -latency1=/msak/latency1.json

# Verify that the msak-server binary can be run.
RUN ./msak-server -h

ENTRYPOINT ["./msak-server"]
