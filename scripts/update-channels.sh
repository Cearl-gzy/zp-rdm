#!/usr/bin/env bash
# 发布后更新分发渠道：Scoop bucket / Homebrew tap manifest 更新并推送，
# 同时在 packaging/winget/<version>/ 生成 winget-pkgs 清单（PR 用）。
# 用法（仓库根目录，Git Bash）: scripts/update-channels.sh v1.11.0
# 前置：gh 已登录（api.github.com 可达即可），git SSH 可推 github.com。
set -euo pipefail

TAG="${1:?用法: scripts/update-channels.sh vX.Y.Z}"
VERSION="${TAG#v}"
REPO="Cearl-gzy/zp-rdm"
BASE="https://github.com/${REPO}/releases/download/${TAG}"
GH="${GH_PATH:-/c/Program Files/GitHub CLI/gh.exe}"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

echo "== 下载资产并计算 sha256 =="
dl() { # $1=资产名
  "$GH" api "repos/${REPO}/releases/assets" --jq ".[] | select(.name==\"$1\") | .id" > "$WORK/id"
  [ -s "$WORK/id" ] || { echo "资产 $1 不存在，Release 未就绪？"; exit 1; }
  "$GH" api "repos/${REPO}/releases/assets/$(cat "$WORK/id")" -H "Accept: application/octet-stream" > "$WORK/$1"
  sha256sum "$WORK/$1" | cut -d' ' -f1
}

SHA_EXE="$(dl zp-rdm.exe)"
SHA_INSTALLER="$(dl zp-rdm-amd64-installer.exe)"
SHA_DARWIN="$(dl zp-rdm-darwin-universal.zip)"
echo "zp-rdm.exe:        $SHA_EXE"
echo "installer:         $SHA_INSTALLER"
echo "darwin zip:        $SHA_DARWIN"

push_if_changed() { # $1=repo dir $2=message
  git -C "$1" add -A
  if git -C "$1" diff --cached --quiet; then
    echo "   无变化，跳过推送"
  else
    git -C "$1" commit -m "$2" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>" -q
    git -C "$1" push origin HEAD
  fi
}

echo "== 更新 Scoop bucket =="
git clone --depth 1 git@github.com:Cearl-gzy/scoop-bucket.git "$WORK/scoop" 2>/dev/null || {
  mkdir -p "$WORK/scoop" && git -C "$WORK/scoop" init -q
  git -C "$WORK/scoop" remote add origin git@github.com:Cearl-gzy/scoop-bucket.git
}
mkdir -p "$WORK/scoop/bucket"
cat > "$WORK/scoop/bucket/zp-rdm.json" <<EOF
{
  "version": "${VERSION}",
  "description": "Lightweight, elegant Redis desktop manager",
  "homepage": "https://github.com/Cearl-gzy/zp-rdm",
  "license": "MIT",
  "architecture": {
    "64bit": {
      "url": "${BASE}/zp-rdm.exe",
      "hash": "${SHA_EXE}"
    }
  },
  "bin": "zp-rdm.exe",
  "shortcuts": [
    [
      "zp-rdm.exe",
      "ZP Redis Manager"
    ]
  ],
  "checkver": {
    "github": "https://github.com/Cearl-gzy/zp-rdm"
  },
  "autoupdate": {
    "architecture": {
      "64bit": {
        "url": "https://github.com/Cearl-gzy/zp-rdm/releases/download/v\$version/zp-rdm.exe"
      }
    }
  }
}
EOF
[ -f "$WORK/scoop/README.md" ] || cat > "$WORK/scoop/README.md" <<'EOF'
# scoop-bucket

Scoop bucket for [ZP Redis Manager](https://github.com/Cearl-gzy/zp-rdm).

```powershell
scoop bucket add zp-rdm https://github.com/Cearl-gzy/scoop-bucket
scoop install zp-rdm
```
EOF
push_if_changed "$WORK/scoop" "zp-rdm ${VERSION}"

echo "== 更新 Homebrew tap =="
git clone --depth 1 git@github.com:Cearl-gzy/homebrew-tap.git "$WORK/brew" 2>/dev/null || {
  mkdir -p "$WORK/brew" && git -C "$WORK/brew" init -q
  git -C "$WORK/brew" remote add origin git@github.com:Cearl-gzy/homebrew-tap.git
}
mkdir -p "$WORK/brew/Casks"
cat > "$WORK/brew/Casks/zp-rdm.rb" <<EOF
cask "zp-rdm" do
  version "${VERSION}"
  sha256 "${SHA_DARWIN}"

  url "https://github.com/Cearl-gzy/zp-rdm/releases/download/v#{version}/zp-rdm-darwin-universal.zip"
  name "ZP Redis Manager"
  desc "Lightweight, elegant Redis desktop manager"
  homepage "https://github.com/Cearl-gzy/zp-rdm"

  app "zp-rdm.app"

  caveats <<~EOS
    The app is not notarized. If Gatekeeper blocks the first launch, run:
      xattr -cr /Applications/zp-rdm.app
  EOS
end
EOF
[ -f "$WORK/brew/README.md" ] || cat > "$WORK/brew/README.md" <<'EOF'
# homebrew-tap

Homebrew tap for [ZP Redis Manager](https://github.com/Cearl-gzy/zp-rdm).

```bash
brew tap Cearl-gzy/tap
brew install --cask zp-rdm
```
EOF
push_if_changed "$WORK/brew" "zp-rdm ${VERSION}"

echo "== 生成 winget 清单（packaging/winget/${VERSION}/）=="
WG="packaging/winget/${VERSION}"
mkdir -p "$WG"
cat > "${WG}/Cearl-gzy.zp-rdm.yaml" <<EOF
# yaml-language-server: \$schema=https://aka.ms/winget-manifest.version.1.10.0.schema.json
PackageIdentifier: Cearl-gzy.zp-rdm
PackageVersion: ${VERSION}
DefaultLocale: en-US
ManifestType: version
ManifestVersion: 1.10.0
EOF
cat > "${WG}/Cearl-gzy.zp-rdm.installer.yaml" <<EOF
# yaml-language-server: \$schema=https://aka.ms/winget-manifest.installer.1.10.0.schema.json
PackageIdentifier: Cearl-gzy.zp-rdm
PackageVersion: ${VERSION}
InstallerType: nullsoft
InstallModes:
  - interactive
  - silent
InstallerSwitches:
  Silent: /S
  SilentWithProgress: /S
UpgradeBehavior: install
Installers:
  - Architecture: x64
    Scope: user
    InstallerUrl: ${BASE}/zp-rdm-amd64-installer.exe
    InstallerSha256: ${SHA_INSTALLER}
ManifestType: installer
ManifestVersion: 1.10.0
EOF
cat > "${WG}/Cearl-gzy.zp-rdm.locale.en-US.yaml" <<EOF
# yaml-language-server: \$schema=https://aka.ms/winget-manifest.defaultLocale.1.10.0.schema.json
PackageIdentifier: Cearl-gzy.zp-rdm
PackageVersion: ${VERSION}
PackageLocale: en-US
Publisher: Cearl-gzy
PublisherUrl: https://github.com/Cearl-gzy
PackageName: ZP Redis Manager
PackageUrl: https://github.com/Cearl-gzy/zp-rdm
License: MIT
LicenseUrl: https://github.com/Cearl-gzy/zp-rdm/blob/main/LICENSE
ShortDescription: Lightweight, elegant Redis desktop manager
Moniker: zp-rdm
Tags:
  - redis
  - database
  - gui
  - client
  - nosql
ManifestType: defaultLocale
ManifestVersion: 1.10.0
EOF
echo "   已生成。提交：microsoft/winget-pkgs 路径 manifests/c/Cearl-gzy/zp-rdm/${VERSION}/"
echo "== 完成 =="
