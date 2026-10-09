FROM ubuntu:26.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        git \
        build-essential \
        bc \
        xz-utils \
        sudo \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --create-home --shell /bin/bash ubuntu \
    && echo 'ubuntu ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/ubuntu \
    && chmod 0440 /etc/sudoers.d/ubuntu

ARG GO_VERSION=1.26.8
ARG NODE_VERSION=24.21.0

RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -o /tmp/go.tgz \
    && tar -C /usr/local -xzf /tmp/go.tgz \
    && rm /tmp/go.tgz \
    && curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-x64.tar.xz" -o /tmp/node.tar.xz \
    && tar -C /usr/local --strip-components=1 -xJf /tmp/node.tar.xz \
    && rm /tmp/node.tar.xz

ENV PATH="/usr/local/go/bin:${PATH}" \
    GOPATH="/home/ubuntu/go" \
    GOCACHE="/home/ubuntu/.cache/go-build" \
    GOMODCACHE="/home/ubuntu/go/pkg/mod"

RUN mkdir -p /home/ubuntu/go /home/ubuntu/.cache/go-build \
    && chown -R ubuntu:ubuntu /home/ubuntu

USER ubuntu
WORKDIR /home/ubuntu
