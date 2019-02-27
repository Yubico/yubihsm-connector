FROM golang:1.9.4-stretch AS build

COPY . /usr/lib/src/yubihsm-connector

RUN ls -la /usr/lib/src/yubihsm-connector

RUN apt-get update -y && apt-get dist-upgrade -y

RUN apt-get install -y curl \
		git \
		pkg-config \
		build-essential \
		libusb-1.0.0-dev

WORKDIR /usr/lib/src/yubihsm-connector

RUN go get github.com/constabulary/gb/...

RUN pwd

RUN ls

RUN make rebuild


FROM debian:stretch-slim

RUN apt-get update -y && apt-get dist-upgrade -y

RUN apt-get install -y libusb-1.0.0

COPY --from=build /usr/lib/src/yubihsm-connector/bin/yubihsm-connector /usr/local/bin/

WORKDIR yubihsm-connector

ENV YUBIHSM_CONNECTOR_LISTEN=0.0.0.0:12345

ENTRYPOINT ["yubihsm-connector"]
CMD ["-d"]
