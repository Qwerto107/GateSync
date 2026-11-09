#!/usr/bin/env bash
set -euo pipefail

cd release-assets
archives=()
for platform in linux-amd64 linux-arm64 linux-armv6 linux-armv5 linux-loong64 windows-amd64 darwin-arm64; do
  extension=tar.gz
  if [[ "$platform" == windows-amd64 ]]; then extension=zip; fi
  archives+=("gatesync-$platform.$extension")
done
for archive in "${archives[@]}"; do
  test -s "$archive"
  test -s "$archive.sha256"
done
sha256sum -c ./*.sha256
tag="${GITHUB_REF_NAME:?缺少版本标签}"
repository="${GH_REPO,,}"
prerelease=false
latest=true
if [[ "$tag" == *-* ]]; then prerelease=true; latest=false; fi
printf '%s\n\n' 'GateSync 完整程序内嵌前端，支持 Linux amd64、arm64、armv6、armv5、loong64，Windows amd64 和 macOS arm64。Windows 使用 ZIP，其余平台使用 tar.gz，每份压缩包附 SHA-256 校验文件。' > release-notes.md
printf '源码提交：%s\n\n统一镜像：`ghcr.io/%s:%s`\n\n' "$GITHUB_SHA" "$repository" "${tag#v}" >> release-notes.md
printf '镜像支持五种 Linux 架构。[二进制启动说明](https://github.com/%s/blob/%s/docs/binary.md) · [容器部署说明](https://github.com/%s/blob/%s/docs/releasing.md)\n' \
  "$GH_REPO" "$tag" "$GH_REPO" "$tag" >> release-notes.md

# 先建草稿，全部附件上传并核对服务器上的实际内容后才公开。
if ! gh release view "$tag" --json tagName > /dev/null 2>&1; then
  gh release create "$tag" --verify-tag --draft --title "$tag" --generate-notes --notes-file release-notes.md
fi
existing_assets="$(gh release view "$tag" --json assets --jq '.assets[].name')"
for archive in "${archives[@]}"; do
  for asset in "$archive" "$archive.sha256"; do
    if ! grep -Fxq "$asset" <<< "$existing_assets"; then gh release upload "$tag" "$asset"; fi
  done
done
mkdir -p verified-assets
gh release download "$tag" --dir verified-assets --pattern 'gatesync-*'
for archive in "${archives[@]}"; do
  test -s "verified-assets/$archive"
  cmp "$archive.sha256" "verified-assets/$archive.sha256"
done
(cd verified-assets && sha256sum -c ./*.sha256)
gh release edit "$tag" --draft=false --prerelease="$prerelease" --latest="$latest"
printf 'Release: https://github.com/%s/releases/tag/%s\n' "$GH_REPO" "$tag" >> "$GITHUB_STEP_SUMMARY"
