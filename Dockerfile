# SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
#
# SPDX-License-Identifier: Apache-2.0
#

FROM golang:1.25.5-bookworm@sha256:09f53deea14d4019922334afe6258b7b776afc1d57952be2012f2c8c4076db05 AS builder

RUN apt-get update && \
    apt-get -y install --no-install-recommends \
    apt-transport-https \
    ca-certificates \
    gcc \
    cmake \
    autoconf \
    libtool \
    pkg-config \
    libmnl-dev \
    libyaml-dev && \
    apt-get clean

WORKDIR $GOPATH/src/amf

COPY . .
RUN make all

FROM alpine:3.23@sha256:51183f2cfa6320055da30872f211093f9ff1d3cf06f39a0bdb212314c5dc7375 AS amf

LABEL maintainer="Aether SD-Core <dev@lists.aetherproject.org>" \
    description="Aether open source 5G Core Network" \
    version="Stage 3"

ARG DEBUG_TOOLS

RUN apk update && apk add --no-cache -U bash

# Install debug tools ~ 50MB (if DEBUG_TOOLS is set to true)
RUN if [ "$DEBUG_TOOLS" = "true" ]; then \
        apk update && apk add --no-cache -U vim strace net-tools curl netcat-openbsd bind-tools; \
        fi

# Copy executable
COPY --from=builder /go/src/amf/bin/* /usr/local/bin/.
