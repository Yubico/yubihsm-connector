FROM golang:1.17-bullseye AS build

RUN apt-get update -y && \
	apt-get install -y \
		curl \
		git \
		pkg-config \
		build-essential \
		libusb-1.0.0-dev && \
	apt-get clean &&\
	rm -rf /var/lib/apt/lists/*

COPY . /usr/lib/src/yubihsm-connector

WORKDIR /usr/lib/src/yubihsm-connector

RUN make rebuild


FROM debian:bullseye-slim

RUN apt-get update -y && \
	apt-get install -y libusb-1.0.0 && \
	apt-get clean && \
	rm -rf /var/lib/apt/lists/*

COPY --from=build /usr/lib/src/yubihsm-connector/bin/yubihsm-connector /usr/local/bin/

ENV YUBIHSM_CONNECTOR_LISTEN=0.0.0.0:12345

ENTRYPOINT ["yubihsm-connector"]
CMD ["-d"]
