FROM golang:1.22.2-alpine3.19 AS builder

WORKDIR /opt/app
COPY ./go.mod ./
COPY ./go.sum ./
COPY ./main.go ./

RUN go build -o qbittorrent-port-plugin ./main.go 

FROM alpine:3.19.1 AS runner

WORKDIR /opt/app
COPY --from=builder /opt/app/qbittorrent-port-plugin .

ENTRYPOINT [ "/opt/app/qbittorrent-port-plugin" ]