FROM golang:1.22.3-alpine as builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o hariko

FROM alpine:3.19.1
COPY --from=builder /app/hariko /
ENTRYPOINT ["/hariko"]
