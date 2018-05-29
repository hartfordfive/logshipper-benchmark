GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
BINARY_NAME=logshipper-benchmark
BINARY_UNIX=$(BINARY_NAME)
GO_DEP_FETCH=govendor fetch 
BUILD_DIR=build/
GITHASH=$(git rev-parse --verify HEAD)
BUILDDATE=$(date +%Y-%m-%d)
VERSION=0.1.0

all: cleanall buildall

build: 
	$(GOBUILD) -ldflags "-s -w" -a -o ${BUILD_DIR}$(BINARY_NAME) -v .

buildplugins:
	$(GOBUILD) -i -a -v -buildmode=plugin -o modules/filebeat_6_1_1.so shipper/filebeat/filebeat_6_1_1.go
	$(GOBUILD) -i -a -v -buildmode=plugin -o modules/rsyslogd_8_34_0.so shipper/rsyslogd/rsyslogd_8_34_0.go
	$(GOBUILD) -i -a -v -buildmode=plugin -o modules/fluentbit_0_13_1.so shipper/fluentbit/fluentbit_0_13_1.go
	$(GOBUILD) -i -a -v -buildmode=plugin -o modules/nxlog_2_10_2102.so shipper/nxlog/nxlog_2_10_2102.go
	$(GOBUILD) -i -a -v -buildmode=plugin -o modules/logstash_6_1_1.so shipper/logstash/logstash_6_1_1.go

buildall: buildplugins build

test: 
	$(GOTEST) -v ./...

clean: 
	$(GOCLEAN)
	rm -rf ${BUILD_DIR}
	rm -rf modules/*

cleanplugins:
	$(GOCLEAN)
	rm -rf modules/*

cleanall: clean cleanplugins

run:
	$(GOBUILD) -a -v -buildmode=plugin -o modules/filebeat_6_1_1.so shipper/filebeat/filebeat_6_1_1.go
	$(GOBUILD) -a -v -buildmode=plugin -o modules/rsyslogd_8_34_0.so shipper/rsyslogd/rsyslogd_8_34_0.go
	$(GOBUILD) -a -v -buildmode=plugin -o modules/fluentbit_0_13_1.so shipper/fluentbit/fluentbit_0_13_1.go
	$(GOBUILD) -a -v -buildmode=plugin -o modules/nxlog_2_10_2102.so shipper/nxlog/nxlog_2_10_2102.go
	$(GOBUILD) -a -v -buildmode=plugin -o modules/logstash_6_1_1.so shipper/logstash/logstash_6_1_1.go
	mkdir ${BUILD_DIR}tmp/
	$(GOBUILD) -a -o ${BUILD_DIR}$(BINARY_NAME) -v ./...
	./${BUILD_DIR}$(BINARY_NAME)

deps:
	$(GO_DEP_FETCH) github.com/c9s/goprocinfo/linux
	$(GO_DEP_FETCH) github.com/Pallinder/go-randomdata
	$(GO_DEP_FETCH) github.com/mitchellh/go-ps
	$(GO_DEP_FETCH) github.com/shirou/gopsutil


# Cross compilation
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -ldflags "-s -w -X main.GitHash=${GITHASH} -X main.BuildDate=${BUILDDATE} -X main.Version=${VERSION}" -o ${BUILD_DIR}$(BINARY_UNIX) -v
