FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/meeting-booking ./cmd/server

FROM gcr.io/distroless/static-debian12
ENV HTTP_ADDR=:8080
EXPOSE 8080
COPY --from=builder /bin/meeting-booking /meeting-booking
ENTRYPOINT ["/meeting-booking"]
