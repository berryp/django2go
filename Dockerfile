FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY . .

RUN go build -o django2go main.go

FROM alpine:latest
COPY --from=builder /app/django2go /usr/bin/django2go

ENTRYPOINT ["django-sqlc"]
