FROM golang:1.24.4-alpine as builder
ENV CGO_ENABLED 0

WORKDIR /src
COPY . .

RUN go build -o smtprelay ./cmd/smtprelay

FROM alpine
WORKDIR /app
COPY --from=builder /src/smtprelay .
CMD ["/app/smtprelay"]
