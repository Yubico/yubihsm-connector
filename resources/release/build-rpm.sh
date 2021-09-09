#!/usr/bin/env bash
set -e -o pipefail
set -x

PLATFORM=$1

if [ "$PLATFORM" == "centos7" ]; then
  sudo yum -y install centos-release-scl
  sudo yum -y update && sudo yum -y upgrade
  sudo yum -y install devtoolset-7-gcc     \
                  devtoolset-7-gcc-c++ \
                  devtoolset-7-make    \
                  chrpath              \
                  git                  \
                  libusbx-devel        \
                  libseccomp-devel     \
                  rpm-build            \
                  redhat-rpm-config
  . /opt/rh/devtoolset-7/enable

elif [ "$PLATFORM" == "centos8" ]; then
  sudo yum -y install epel-release
  sudo yum -y update && sudo yum -y upgrade

  sudo dnf group -y install "Development Tools"
  sudo dnf config-manager -y --set-enabled powertools

  sudo yum -y install chrpath          \
                  libusbx-devel        \
                  libseccomp-devel

elif [ "${PLATFORM:0:6}" == "fedora" ]; then
  sudo dnf -y update
  sudo dnf -y install gcc binutils git make libusb-devel rpmdevtools
fi


export PATH=$PATH:/usr/local/go/bin:~/go/bin
if [[ ! -x $(command -v go ) ]]; then
  curl -L --max-redirs 2 -o - https://golang.org/dl/go1.17.linux-amd64.tar.gz |\
    sudo tar -C /usr/local -xzvf -
fi

export INPUT=/shared
export OUTPUT=/shared/resources/release/build/$PLATFORM/yubihsm-connector
rm -rf "${OUTPUT}"
mkdir -p "${OUTPUT}"

# These 2 lines can be replaced by the command "rpmdev-setuptree", but this command seems to add macros that force check paths that do not exist
mkdir -p ~/rpmbuild/{BUILD,RPMS,SOURCES,SPECS,SRPMS}
echo '%_topdir %(echo $HOME)/rpmbuild' > ~/.rpmmacros

export RPM_DIR=~/rpmbuild
cp /shared/resources/release/yubihsm-connector.spec $RPM_DIR/SPECS/

go version
rpmbuild -bb $RPM_DIR/SPECS/yubihsm-connector.spec
cp $RPM_DIR/RPMS/x86_64/*.rpm $OUTPUT

LICENSE_DIR="$OUTPUT/share/yubihsm-connector"
mkdir -p $LICENSE_DIR
pushd "/shared" &>/dev/null
  cp -r resources/release/licenses $LICENSE_DIR/
  for lf in $LICENSE_DIR/licenses/*; do
	  chmod 644 $lf
  done

  pushd "$OUTPUT" &>/dev/null
    rm -f "yubihsm-connector-$PLATFORM-amd64.tar.gz"
    tar -C ".." -zcvf "../yubihsm-connector-$PLATFORM-amd64.tar.gz" "yubihsm-connector"
    rm -f *.rpm
    rm -rf licenses
    rm -rf ../yubihsm-connector
  popd &>/dev/null
popd &>/dev/null
