FROM golang:1.21.4-alpine as builder
ENV CGO_ENABLED 0

WORKDIR /src
COPY . .

RUN mkdir build && \
	go build -o ./build/ ./cmd/smtprelay

FROM alpine
WORKDIR /app
COPY --from=builder /src/build/smtprelay .
CMD ["/app/smtprelay"]
