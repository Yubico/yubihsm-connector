FROM debian:stretch-slim

RUN apt-get update -y && apt-get dist-upgrade -y && apt-get install -y libusb-1.0.0

ADD bin/yubihsm-connector /bin

ENV YUBIHSM_CONNECTOR_LISTEN=0.0.0.0:12345

CMD ["/bin/yubihsm-connector", "-d"]
