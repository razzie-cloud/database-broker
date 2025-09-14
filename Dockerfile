FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/database-broker ./cmd/database-broker

FROM alpine:latest
COPY --from=build /out/database-broker /database-broker
EXPOSE 8080
ENTRYPOINT ["/database-broker"]
