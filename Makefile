# yubihsm-connector

MAKEFLAGS += -s
MAKEFLAGS += --no-builtin-rules
.SUFFIXES:

all: build

build:
	@gb generate ${GB_GEN_FLAGS}
	@gb build ${GB_BUILD_FLAGS}

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
	@go fmt ./src/...

vet:
	@go vet ./src/...

test: vet
	@gb test ${GB_BUILD_FLAGS} -v

docker-build:
	@docker build -f Dockerfile.build -t yubico:yubihsm-connector-build .

docker-build-run: docker-build
	@docker run --rm -v ${PWD}:/yubihsm-connector yubico:yubihsm-connector-build

docker: docker-build
	@docker build -f Dockerfile -t yubico:yubihsm-connector .

docker-run: docker
	@docker run --rm --privileged -v /dev/bus/usb/:/dev/bus/usb/ -p 12345:12345 yubico:yubihsm-connector

clean:
	@rm -rf bin/* pkg/* src/yubihsm-connector/*.syso \
		src/yubihsm-connector/versioninfo.json \
		src/yubihsm-connector/version.go

.PHONY: all build fmt vet test clean version
