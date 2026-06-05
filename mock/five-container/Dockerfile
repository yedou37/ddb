FROM alpine:3.20

RUN apk add --no-cache \
    bash \
    curl \
    ca-certificates \
    iproute2 \
    net-tools \
    procps \
    jq \
    bind-tools \
    iputils \
    vim

WORKDIR /work

RUN mkdir -p /data /logs /workspace

CMD ["sleep", "infinity"]
