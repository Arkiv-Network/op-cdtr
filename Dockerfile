# syntax=docker/dockerfile:1

FROM gcr.io/distroless/static-debian12:latest

ARG TARGETPLATFORM

WORKDIR /app

COPY ${TARGETPLATFORM}/op-cdtr /app/

ENTRYPOINT ["/app/op-cdtr"]
