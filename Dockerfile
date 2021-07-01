# Copyright 2019-present Open Networking Foundation
#
# SPDX-License-Identifier: Apache-2.0
#

FROM golang:1.14.4-stretch AS builder

LABEL maintainer="ONF <omec-dev@opennetworking.org>"

#RUN apt remove cmdtest yarn
RUN apt-get update
RUN apt-get -y install apt-transport-https ca-certificates
RUN curl -sS https://dl.yarnpkg.com/debian/pubkey.gpg > pubkey.gpg
RUN apt-key add pubkey.gpg
RUN curl -sL https://deb.nodesource.com/setup_10.x | bash -
RUN echo "deb https://dl.yarnpkg.com/debian/ stable main" |  tee /etc/apt/sources.list.d/yarn.list
RUN apt-get update
RUN apt-get -y install gcc cmake autoconf libtool pkg-config libmnl-dev libyaml-dev  nodejs yarn
RUN apt-get clean

ENV GITHUB_TOKEN=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
RUN git config --global url."https://$GITHUB_TOKEN@github.com".insteadOf "https://github.com"
ENV GO111MODULE=on
ENV GOSUMDB=off
#RUN go get -u github.com/omec-project/config5g

RUN cd $GOPATH/src && mkdir -p amf
COPY . $GOPATH/src/amf
RUN cd $GOPATH/src/amf \
    && make all

FROM alpine:3.8 as amf

LABEL description="ONF open source 5G Core Network" \
    version="Stage 3"

ARG DEBUG_TOOLS

# Install debug tools ~ 100MB (if DEBUG_TOOLS is set to true)
RUN apk update
RUN apk add -U vim strace net-tools curl netcat-openbsd bind-tools

# Set working dir
WORKDIR /free5gc
RUN mkdir -p amf/

# Copy executable and default certs
COPY --from=builder /go/src/amf/bin/* ./amf
WORKDIR /free5gc/amf
# Exposed ports
EXPOSE 29518
