FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o temujin ./cmd/temujin

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /app/temujin .
COPY souls/ ./souls/
EXPOSE 7891
CMD ["./temujin", "serve", "7891"]
