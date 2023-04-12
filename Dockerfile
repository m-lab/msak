# syntax=docker/dockerfile:1
FROM golang:1.20-alpine AS build
RUN apk add gcc git linux-headers musl-dev
WORKDIR /msak

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY ./ ./
RUN ./build.sh

FROM alpine
WORKDIR /msak
COPY --from=build /msak/msak-server /msak/

ENTRYPOINT ["./msak-server"]
