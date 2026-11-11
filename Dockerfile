FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/database-broker ./cmd/database-broker
RUN CGO_ENABLED=0 go build -o /out/broker-token ./cmd/broker-token

FROM alpine:latest
COPY --from=build /out/database-broker /database-broker
COPY --from=build /out/broker-token /broker-token
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=3s --start-period=10s CMD wget -q -O /dev/null http://127.0.0.1:${SERVICE_PORT:-8080}/readyz || exit 1
ENTRYPOINT ["/database-broker"]
