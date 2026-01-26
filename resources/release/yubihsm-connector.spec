Name:		yubihsm-connector
Version:	3.0.7
Release:	1%{?dist}
Summary:	USB to HTTP bridge for the YubiHSM

License:	Apache 2.0
URL:        https://github.com/Yubico/yubihsm-connector


%description
This package contains a connector allowing for communication with a YubiHSM 2 device using HTTP protocol.

%prep
cd %{_builddir}
rm -rf *
cp -r $INPUT/* .


%build
make GB_BUILD_FLAGS="-tags disable_seccomp"
strip --strip-all bin/yubihsm-connector

#Would be nice to use %license, but that macro does not seem to work on Centos, so the license needs to be installed manually

%install
mkdir -p %{buildroot}/%{_bindir}
install -m 0755 bin/%{name} %{buildroot}/%{_bindir}/%{name}
mkdir -p %{buildroot}/%{_prefix}/share/licenses/%{name}
install -m 0644 LICENSE %{buildroot}/%{_prefix}/share/licenses/%{name}



%files
%{_bindir}/%{name}
%{_prefix}/share/licenses/%{name}/LICENSE

%changelog
* Sat Jan 17 2026 Aveen Ismail <aveen.ismail@yubico.com> - 3.0.6
- Releasing 3.0.6
