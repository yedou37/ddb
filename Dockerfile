FROM --platform=$BUILDPLATFORM golang:1.25.8-alpine AS build

ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -o /out/ddb-server ./cmd/server && \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -o /out/ddb-cli ./cmd/cli

FROM --platform=$TARGETPLATFORM alpine:3.20

RUN apk add --no-cache ca-certificates && \
    adduser -D -h /app app

WORKDIR /app

COPY --from=build /out/ddb-server /usr/local/bin/ddb-server
COPY --from=build /out/ddb-cli /usr/local/bin/ddb-cli

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

ENTRYPOINT ["ddb-server"]
