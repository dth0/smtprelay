FROM golang:1.19.1-alpine as builder

WORKDIR /src
COPY . .

ENV CGO_ENABLED 0

RUN mkdir build \
	&& go build -o ./build/ ./cmd/smtprelay

FROM alpine
COPY --from=builder /src/build/* /usr/local/bin/
CMD ["/usr/local/bin/smtprelay"]
