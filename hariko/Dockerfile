FROM golang:1.22.3-alpine as builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o hariko

FROM alpine:3.19.1
COPY --from=builder /app/hariko /
SHELL ["/bin/ash", "-o", "pipefail", "-c"]
RUN export HELM_DRIVER=configmap \
 && apk add --no-cache bash=~5.2.21 curl=~8.5.0 openssl=~3.1.6 \
 && curl -LSfs https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
ENTRYPOINT ["/hariko"]
