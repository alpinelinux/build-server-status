FROM registry.alpinelinux.org/alpine/infra/docker/golang:3.22 AS builder

COPY --chown=build ./ /src

WORKDIR /src

RUN redo build-server-status

FROM alpine:edge

COPY --from=builder /src/build-server-status /build-server-status

EXPOSE 8080

CMD ["/build-server-status"]
