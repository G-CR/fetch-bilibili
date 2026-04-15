ARG GO_IMAGE=golang:1.22-alpine
ARG ALPINE_IMAGE=alpine:3.20

FROM ${GO_IMAGE} AS build

WORKDIR /src

# 缓存依赖
COPY go.mod ./
RUN go mod download

COPY . .

# 构建可执行文件
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server ./cmd/server

FROM ${ALPINE_IMAGE}

RUN apk add --no-cache ffmpeg
RUN addgroup -S app && adduser -S app -G app
WORKDIR /app

COPY --from=build /out/server /app/server
COPY configs/config.example.yaml /app/config.yaml

USER app
EXPOSE 8080

ENTRYPOINT ["/app/server"]
