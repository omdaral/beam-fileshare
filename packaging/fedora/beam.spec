# Beam file sharing — RPM spec (Fedora/openSUSE).
# Build on Fedora:  ./packaging/fedora/make-tarball.sh <ver> && rpmbuild -bb packaging/fedora/beam.spec --define "ver <ver>"
# (rpmbuild needs: sudo dnf install rpm-build golang; COPR steps in packaging/README.md)
# TODO(packaging): set real Packager identity before publishing.
Name:           beam-fileshare
Version:        %{ver}
Release:        1%{?dist}
Summary:        Direct file sharing between devices over a private network
License:        LicenseRef-omdaral-Beam-NonCommercial-1.0
# URL: TODO project homepage (omitted: rpm tags take a single token)
Source0:        beam-%{version}.tar.gz

BuildRequires:  golang >= 1.21
Requires:       curl, xdg-utils
Recommends:     libnotify

%description
 Beam shares files between phones and computers on the same network,
 no internet and no accounts needed. Single static binary with an
 offline Arabic/English web UI. Shared files live in ~/Downloads/Beam;
 no config or log files are written anywhere.

%prep
%autosetup -n beam-%{version}

%build
export CGO_ENABLED=0 GOPROXY=off
go -C goserver vet ./...
go -C goserver test -count=1 ./...
go -C goserver build -trimpath -ldflags="-s -w -X fileshare.AppVersion=%{version}" -o ../Beam-linux ./cmd/beam

%install
install -Dm755 Beam-linux %{buildroot}%{_libexecdir}/beam/Beam
install -Dm755 packaging/common/beam-wrapper.sh %{buildroot}%{_bindir}/beam
install -Dm644 packaging/common/beam.1 %{buildroot}%{_mandir}/man1/beam.1
install -Dm644 packaging/fedora/beam-fileshare.desktop %{buildroot}%{_datadir}/applications/beam-fileshare.desktop
for s in 16 22 24 32 48 64 128 256; do
  install -Dm644 packaging/icons/beam_${s}.png \
    %{buildroot}%{_datadir}/icons/hicolor/${s}x${s}/apps/beam.png
done

%files
%{_libexecdir}/beam/Beam
%{_bindir}/beam
%{_mandir}/man1/beam.1*
%{_datadir}/applications/beam-fileshare.desktop
%{_datadir}/icons/hicolor/*/apps/beam.png

%changelog
* Sun Sep 13 2026 Beam Project <beam-fileshare@localhost> - 1.6.0-1
- Upstream 1.6.0 (TODO: packager identity + date per release)
