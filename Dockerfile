FROM golang:1.24-alpine AS builder

ARG VERSION=dev

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X github.com/huey1in/KiroClaim/utils.AppVersion=${VERSION}" -o /kiroclaim .

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata docker-cli docker-cli-compose \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo "Asia/Shanghai" > /etc/timezone

WORKDIR /app

COPY --from=builder /kiroclaim .
COPY static/ ./static/

EXPOSE 9527
VOLUME ["/app/data", "/app/logs"]

ENTRYPOINT ["./kiroclaim"]
