FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/database-broker ./cmd/database-broker
RUN CGO_ENABLED=0 go build -o /out/broker-token ./cmd/broker-token
RUN CGO_ENABLED=0 go build -o /out/broker-start ./cmd/broker-start

FROM alpine:latest
COPY --from=build /out/database-broker /database-broker
COPY --from=build /out/broker-token /broker-token
COPY --from=build /out/broker-start /broker-start
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=3s --start-period=10s CMD port=${SERVICE_PORT:-8080}; if [ -n "$TLS_CERT_FILE" ]; then port=${HEALTH_PORT:-8081}; fi; wget -q -O /dev/null http://127.0.0.1:$port/readyz || exit 1
ENTRYPOINT ["/database-broker"]
