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

RUN ./msak-server -h
ENTRYPOINT ["./msak-server"]
