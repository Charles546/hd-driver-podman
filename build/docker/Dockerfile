ARG HD_VERSION="4.0.0-rc-ddaf102"

FROM golang:1.26.1-alpine AS build

WORKDIR /build/hd-driver-podman

RUN apk add --no-cache git gcc libc-dev gpgme-dev pkgconf btrfs-progs-dev
COPY go.mod go.sum cmd/ ./
RUN go install -v ./...

FROM docker.io/charleshoney/honeydipper:$HD_VERSION

RUN apk add --no-cache gpgme btrfs-progs

WORKDIR /opt/honeydipper/drivers/builtin
COPY --from=build /go/bin/* ./
