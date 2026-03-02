FROM golang:1.26.0-alpine AS build

WORKDIR /build/hd-driver-podman

RUN apk add --no-cache git gcc libc-dev gpgme-dev pkgconf btrfs-progs-dev
COPY ./ ./
RUN go install -v ./...

FROM docker.io/charleshoney/honeydipper:4.0.0-rc-f42a077

RUN apk add --no-cache gpgme btrfs-progs

WORKDIR /opt/honeydipper/drivers/builtin
COPY --from=build /go/bin/* ./
