#!/usr/bin/env bash
# Publish the .deb files in $1 (default dist/) to the dorkitude/webctl-apt
# repository, which GitHub Pages serves at https://dorkitude.github.io/webctl-apt.
# Needs: dpkg-dev, apt-utils, gpg; APT_GPG_PRIVATE_KEY (armored) and
# TAP_GITHUB_TOKEN (push access to the apt repo) in the environment.
set -euo pipefail

dist=$(cd "${1:-dist}" && pwd)
repo=https://x-access-token:${TAP_GITHUB_TOKEN}@github.com/dorkitude/webctl-apt.git
work=$(mktemp -d)

if ! command -v dpkg-scanpackages >/dev/null || ! command -v apt-ftparchive >/dev/null; then
  sudo apt-get update -qq && sudo apt-get install -y -qq dpkg-dev apt-utils
fi

gpg --batch --import <<<"$APT_GPG_PRIVATE_KEY"
keyid=$(gpg --list-secret-keys --with-colons | awk -F: '$1=="sec"{print $5; exit}')

git clone -q "$repo" "$work/apt"
cd "$work/apt"
mkdir -p pool/main/w/webctl
cp "$dist"/*.deb pool/main/w/webctl/

for arch in amd64 arm64; do
  mkdir -p "dists/stable/main/binary-$arch"
  dpkg-scanpackages --multiversion --arch "$arch" pool/ > "dists/stable/main/binary-$arch/Packages"
  gzip -9 -k -f "dists/stable/main/binary-$arch/Packages"
done

apt-ftparchive \
  -o APT::FTPArchive::Release::Origin=webctl \
  -o APT::FTPArchive::Release::Label=webctl \
  -o APT::FTPArchive::Release::Suite=stable \
  -o APT::FTPArchive::Release::Codename=stable \
  -o APT::FTPArchive::Release::Architectures="amd64 arm64" \
  -o APT::FTPArchive::Release::Components=main \
  -o APT::FTPArchive::Release::Description="webctl apt repository" \
  release dists/stable > dists/stable/Release

gpg --batch --yes --default-key "$keyid" -abs -o dists/stable/Release.gpg dists/stable/Release
gpg --batch --yes --default-key "$keyid" --clearsign -o dists/stable/InRelease dists/stable/Release
gpg --armor --export "$keyid" > key.gpg

git config user.name goreleaser
git config user.email kyle@kylewild.com
git add -A
git commit -qm "Publish $(basename "$dist"/webctl_*_amd64.deb .deb | sed 's/_amd64//')" || echo "nothing to publish"
git push -q origin HEAD:main
