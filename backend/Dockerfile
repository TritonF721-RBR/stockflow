# Stage 1: Build binary
FROM golang:alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o stockflow-api .

# Stage 2: Runtime image minimal
FROM alpine:3.20

WORKDIR /app
COPY --from=builder /app/stockflow-api .

EXPOSE 8080

CMD ["./stockflow-api"]