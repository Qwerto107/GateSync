#!/usr/bin/env bash
set -euo pipefail

case "${RELEASE_PLATFORM:?缺少平台}" in
  linux-amd64|linux-arm64|linux-armv6|linux-armv5|linux-loong64|windows-amd64|darwin-arm64) ;;
  *) printf '不支持的平台\n' >&2; exit 1 ;;
esac
target="gatesync-$RELEASE_PLATFORM"
executable=gatesync
if [[ "$GOOS" == windows ]]; then executable=gatesync.exe; fi
epoch="$(git show -s --format=%ct HEAD)"
mkdir -p "bin/release/$target"
go build -tags=with_frontend -trimpath -ldflags='-s -w' -o "bin/release/$target/$executable" ./cmd/gatesync
chmod 0755 "bin/release/$target/$executable"
cp docs/binary.md "bin/release/$target/README.md"
# 压缩包固定文件时间、排序和所有者，重跑时可核对同一提交的附件。
if [[ "$GOOS" == windows ]]; then
  archive="$target.zip"
  find "bin/release/$target" -exec touch -d "@$epoch" {} +
  (cd bin/release && zip -X -r "$archive" "$target")
else
  archive="$target.tar.gz"
  tar --sort=name --mtime="@$epoch" --owner=0 --group=0 --numeric-owner -czf "bin/release/$archive" -C bin/release "$target"
fi
(cd bin/release && sha256sum "$archive" > "$archive.sha256")
