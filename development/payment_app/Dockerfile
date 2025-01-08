# Builder image
FROM golang:1.23-alpine AS builder

WORKDIR /app

# No dependencies yet
## COPY ./go.mod ./go.sum ./
## RUN go mod download

COPY . .
RUN go build -o /app/payment-app main.go


# Final image
FROM alpine:3.18

WORKDIR /app
COPY --from=builder /app/payment-app /usr/local/bin/payment-app

COPY ./users.csv /app/users.csv

CMD ["/usr/local/bin/payment-app"]
