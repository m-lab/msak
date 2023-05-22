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

# Generate msak's JSON schema.
RUN /msak/generate-schema -ndt8=/msak/ndt8.json

# Verify that the msak-server binary can be run.
RUN ./msak-server -h

ENTRYPOINT ["./msak-server"]
