# syntax=docker/dockerfile:1
FROM node:22-alpine AS fe
WORKDIR /ui
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories
COPY frontend-user/package.json frontend-user/package-lock.json* ./
RUN npm install --registry=https://registry.npmmirror.com
COPY frontend-user/ ./
RUN npm run build

FROM golang:1.25-alpine AS be
WORKDIR /src
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories
ENV GOPROXY=https://goproxy.cn,direct
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server

FROM alpine:3.21
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
 && apk add --no-cache wget tzdata ca-certificates \
 && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime
ENV TZ=Asia/Shanghai \
    MDL_ADDR=:8080 \
    MDL_DATA_DIR=/data \
    MDL_STATIC_DIR=/app/static \
    MDL_LOG_LEVEL=info
WORKDIR /app
COPY --from=be /out/server /app/server
COPY --from=fe /ui/dist /app/static
EXPOSE 8080
CMD ["/app/server"]
