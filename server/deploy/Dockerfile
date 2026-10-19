# 揭东一初联机大厅服务端镜像
FROM golang:1.21-alpine AS build
RUN apk add --no-cache gcc musl-dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o /out/jdyc-hub-server .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/jdyc-hub-server /usr/local/bin/jdyc-hub-server
ENV JDYC_LISTEN=:7100 JDYC_DB=/data/jdyc.db
VOLUME /data
EXPOSE 7100
ENTRYPOINT ["/usr/local/bin/jdyc-hub-server"]
