# Copyright 2019-present Open Networking Foundation
#
# SPDX-License-Identifier: Apache-2.0

GO_BIN_PATH = bin
GO_SRC_PATH = ./
C_BUILD_PATH = build
ROOT_PATH = $(shell pwd)

NF = $(GO_NF)
GO_NF = amf

NF_GO_FILES = $(shell find $(GO_SRC_PATH)/$(%) -name "*.go" ! -name "*_test.go")
NF_GO_FILES_ALL = $(shell find $(GO_SRC_PATH)/$(%) -name "*.go")

VERSION = $(shell git describe --tags)
BUILD_TIME = $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
COMMIT_HASH = $(shell git submodule status | grep $(GO_SRC_PATH)/$(@F) | awk '{print $$(1)}' | cut -c1-8)
COMMIT_TIME = $(shell cd $(GO_SRC_PATH) && git log --pretty="%ai" -1 | awk '{time=$$(1)"T"$$(2)"Z"; print time}')
LDFLAGS = -X github.com/free5gc/version.VERSION=$(VERSION) \
          -X github.com/free5gc/version.BUILD_TIME=$(BUILD_TIME) \
          -X github.com/free5gc/version.COMMIT_HASH=$(COMMIT_HASH) \
          -X github.com/free5gc/version.COMMIT_TIME=$(COMMIT_TIME)

.PHONY: $(NF) clean

.DEFAULT_GOAL: nfs

nfs: $(NF)

all: $(NF)

$(GO_NF): % : $(GO_BIN_PATH)/%

$(GO_BIN_PATH)/%: %.go $(NF_GO_FILES)
# $(@F): The file-within-directory part of the file name of the target.
	@echo "Start building $(@F)...."
	cd $(GO_SRC_PATH)/ && \
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(ROOT_PATH)/$@ $(@F).go

vpath %.go $(addprefix $(GO_SRC_PATH)/, $(GO_NF))

test: $(NF_GO_FILES_ALL) 
	@echo "Start building $(@F)...."
	cd $(GO_SRC_PATH)/ && \
	CGO_ENABLED=0 go test -ldflags "$(LDFLAGS)" -o $(ROOT_PATH)/$@ 

clean:
	rm -rf $(addprefix $(GO_BIN_PATH)/, $(GO_NF))
	rm -rf $(addprefix $(GO_SRC_PATH)/, $(addsuffix /$(C_BUILD_PATH), $(C_NF)))

