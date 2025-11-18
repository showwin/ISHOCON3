# Builder image
FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY . .
RUN go build -o /usr/local/bin/payment-app

CMD ["/usr/local/bin/payment-app"]
