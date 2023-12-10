# SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
#
# SPDX-License-Identifier: Apache-2.0
#

FROM golang:1.21.5-bookworm AS builder

LABEL maintainer="ONF <omec-dev@opennetworking.org>"

RUN echo "deb http://archive.debian.org/debian stretch main" > /etc/apt/sources.list
RUN apt-get update
RUN apt-get -y install apt-transport-https ca-certificates
RUN apt-get update
RUN apt-get -y install gcc cmake autoconf libtool pkg-config libmnl-dev libyaml-dev
RUN apt-get clean

RUN cd $GOPATH/src && mkdir -p amf
COPY . $GOPATH/src/amf
RUN cd $GOPATH/src/amf \
    && make all

FROM alpine:3.18 as amf

LABEL description="ONF open source 5G Core Network" \
    version="Stage 3"

ARG DEBUG_TOOLS

# Install debug tools ~ 100MB (if DEBUG_TOOLS is set to true)
RUN apk update
RUN apk add -U vim strace net-tools curl netcat-openbsd bind-tools bash

# Set working dir
WORKDIR /free5gc
RUN mkdir -p amf/

# Copy executable and default certs
COPY --from=builder /go/src/amf/bin/* ./amf
WORKDIR /free5gc/amf
