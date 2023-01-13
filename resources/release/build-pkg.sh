#!/usr/bin/env bash
set -e -o pipefail
set -x

PLATFORM=$1

export DEBIAN_FRONTEND=noninteractive

sudo apt-get update && sudo  apt-get dist-upgrade -y
sudo apt-get install -y build-essential libusb-1.0.0-dev pkg-config chrpath git curl


export PATH=$PATH:/usr/local/go/bin:~/go/bin
if [[ ! -x $(command -v go ) ]]; then
  curl -L --max-redirs 2 -o - https://golang.org/dl/go1.17.linux-amd64.tar.gz |\
    sudo tar -C /usr/local -xzvf -
fi
if [[ ! -x $(command -v go-bin-deb) ]]; then
  curl -L -o go-bin-deb.dpkg https://github.com/mh-cbon/go-bin-deb/releases/download/0.0.19/go-bin-deb-amd64.deb
  sudo dpkg -i go-bin-deb.dpkg
  sudo apt-get install --fix-missing
fi


export INPUT=/shared/
export OUTPUT=/shared/resources/release/build/$PLATFORM/yubihsm-connector
rm -rf $OUTPUT
mkdir -p $OUTPUT

pushd "/tmp" &>/dev/null
  rm -rf yubihsm-connector
  git clone "$INPUT" yubihsm-connector
  pushd "yubihsm-connector" &>/dev/null
    make
    strip --strip-all bin/yubihsm-connector
    version=`bin/yubihsm-connector version`
    go-bin-deb generate -f deb/deb.json -a amd64 --version=${version}-1
    cp *.deb "${OUTPUT}"
  popd &>/dev/null
popd &>/dev/null

LICESE_DIR="$OUTPUT/share/yubihsm-connector"
mkdir -p $LICESE_DIR
pushd "/shared" &>/dev/null
  cp -r resources/release/licenses $LICESE_DIR/
  for lf in $LICESE_DIR/licenses/*; do
	  chmod 644 $lf
  done

  pushd "$OUTPUT" &>/dev/null
    rm -f yubihsm-connector-$PLATFORM-amd64.tar.gz
    tar -C .. -zcvf ../yubihsm-connector-$PLATFORM-amd64.tar.gz yubihsm-connector
    rm -f *.deb
    rm -rf licenses
    rm -rf ../yubihsm-connector
  popd &>/dev/null
popd &>/dev/null