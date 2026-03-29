# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /geo-energy-otel

FROM alpine:3.18.2

WORKDIR /

COPY --from=build /geo-energy-otel /geo-energy-otel

ENTRYPOINT [ "/geo-energy-otel" ]
