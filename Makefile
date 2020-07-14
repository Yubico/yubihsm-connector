# yubihsm-connector

MAKEFLAGS += -s
MAKEFLAGS += --no-builtin-rules
.SUFFIXES:

all: build

gen:
	@go generate

build: gen
	@go build -o bin/yubihsm-connector

rebuild: clean build

install: build
	install bin/yubihsm-connector /usr/local/bin

cert:
	@./tools/generate-certificate

run: build
	@./bin/yubihsm-connector -d

srun: cert build
	@./bin/yubihsm-connector -d --cert=var/cert.crt --key=var/cert.key

fmt:
	@go fmt

vet: gen
	@go vet

test: vet
	@go test -v

docker-clean:
	@docker rmi yubico/yubihsm-connector

docker-build:
	@docker build -t yubico/yubihsm-connector -f Dockerfile .

docker-run:
	@docker run --rm -it --privileged -v ${PWD}:/yubihsm-connector -v /dev/bus/usb/:/dev/bus/usb/ -p 12345:12345 yubico/yubihsm-connector

clean:
	@rm -rf bin/* pkg/* *.syso versioninfo.json version.go

.PHONY: all build fmt vet test clean version
