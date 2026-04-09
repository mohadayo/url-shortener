FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
COPY static/ ./static/
RUN go build -o url-shortener-api .

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/url-shortener-api .
EXPOSE 8080
CMD ["./url-shortener-api"]
