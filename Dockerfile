FROM golang:1.25.8-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/dbd-server ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/dbd-cli ./cmd/cli

FROM alpine:3.20

RUN apk add --no-cache ca-certificates && \
    adduser -D -h /app app

WORKDIR /app

COPY --from=build /out/dbd-server /usr/local/bin/dbd-server
COPY --from=build /out/dbd-cli /usr/local/bin/dbd-cli

RUN mkdir -p /data && chown -R app:app /data /app

USER app

ENV NODE_ID=node1 \
    HTTP_ADDR=:8080 \
    RAFT_ADDR=:7000 \
    RAFT_DIR=/data/raft \
    DB_PATH=/data/data.db \
    ETCD_ADDR= \
    BOOTSTRAP=false \
    REJOIN=false \
    JOIN_ADDR=

EXPOSE 8080 7000

ENTRYPOINT ["dbd-server"]
