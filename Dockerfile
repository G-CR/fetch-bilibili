# syntax=docker/dockerfile:1
FROM golang:1.22-alpine AS build

WORKDIR /src

# 缓存依赖
COPY go.mod ./
RUN go mod download

COPY . .

# 构建可执行文件
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server ./cmd/server

FROM alpine:3.20

RUN addgroup -S app && adduser -S app -G app
WORKDIR /app

COPY --from=build /out/server /app/server
COPY configs/config.example.yaml /app/config.yaml

USER app
EXPOSE 8080

ENTRYPOINT ["/app/server"]
