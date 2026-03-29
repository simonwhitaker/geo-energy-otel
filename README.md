# geo-energy-otel

A Go application that periodically queries the [Geotogether](https://geotogether.com/) smart meter API and exports energy metrics via OpenTelemetry (OTLP gRPC) to any compatible backend.

## Getting started

```sh
export GEO_USERNAME="me@example.com"
export GEO_PASSWORD="<geotogether.com password here>"
export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4317"
export OTEL_HOSTNAME="my-host" # optional, defaults to localhost

go run .
```

## Using Docker

Export the environment variables as above (e.g. by adding a `.env` file), then:

```sh
docker compose up
```

A `docker-compose-hyperdx.yml` file is included for running [HyperDX](https://www.hyperdx.io/) locally as an OTLP backend for testing.
