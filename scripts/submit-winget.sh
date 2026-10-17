#!/usr/bin/env bash
# 把 packaging/winget/<version>/ 的三份清单提交到 microsoft/winget-pkgs。
# 全程走 GitHub API（fork 分支 + git blobs/trees/commits + PR），不需要 clone 5GB 仓库。
# 涉及公共资源写入（fork 分支、对上游开 PR），按约定由仓库所有者本人执行：
#   bash scripts/submit-winget.sh v1.11.0
# 前置：gh 已登录；已执行过一次 gh repo fork microsoft/winget-pkgs --clone=false；
#       scripts/update-channels.sh <tag> 已生成 packaging/winget/<version>/ 清单。
# 只依赖 gh（--jq 为 gh 内置）与 base64，不需要独立 jq。
set -euo pipefail

TAG="${1:?用法: scripts/submit-winget.sh vX.Y.Z}"
VERSION="${TAG#v}"
UPSTREAM="microsoft/winget-pkgs"
OWNER="Cearl-gzy"
FORK="${OWNER}/winget-pkgs"
BRANCH="zp-rdm-${VERSION}"
PKG_DIR="packaging/winget/${VERSION}"
TREE_PREFIX="manifests/c/Cearl-gzy/zp-rdm/${VERSION}"
GH="${GH_PATH:-/c/Program Files/GitHub CLI/gh.exe}"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

[ -d "$PKG_DIR" ] || { echo "缺少 $PKG_DIR，先跑 scripts/update-channels.sh $TAG"; exit 1; }

echo "== 检查 fork =="
"$GH" api "repos/${FORK}" --jq '.full_name' > /dev/null || {
  echo "fork 不存在，请先执行：gh repo fork ${UPSTREAM} --clone=false"; exit 1;
}

echo "== 读取上游 master =="
BASE_SHA="$("$GH" api "repos/${UPSTREAM}/git/refs/heads/master" --jq '.object.sha')"
echo "   master: ${BASE_SHA:0:10}"

echo "== 上传 blobs =="
ENTRIES="" # tree 条目 JSON 片段（逗号拼接）
for f in "${PKG_DIR}"/*.yaml; do
  name="$(basename "$f")"
  b64="$(base64 -w0 "$f")"
  blob_sha="$("$GH" api -X POST "repos/${FORK}/git/blobs" \
    -f content="$b64" -f encoding="base64" --jq '.sha')"
  echo "   ${name}: ${blob_sha:0:10}"
  [ -n "$ENTRIES" ] && ENTRIES="${ENTRIES},"
  ENTRIES="${ENTRIES}{\"path\":\"${TREE_PREFIX}/${name}\",\"mode\":\"100644\",\"type\":\"blob\",\"sha\":\"${blob_sha}\"}"
done

echo "== 建 tree / commit / 分支 =="
printf '{"base_tree":"%s","tree":[%s]}' "$BASE_SHA" "$ENTRIES" > "$WORK/tree.json"
TREE_SHA="$("$GH" api -X POST "repos/${FORK}/git/trees" --input "$WORK/tree.json" --jq '.sha')"

COMMIT_MSG="New version: Cearl-gzy.zp-rdm version ${VERSION}"
printf '{"message":"%s","tree":"%s","parents":["%s"]}' "$COMMIT_MSG" "$TREE_SHA" "$BASE_SHA" > "$WORK/commit.json"
COMMIT_SHA="$("$GH" api -X POST "repos/${FORK}/git/commits" --input "$WORK/commit.json" --jq '.sha')"
echo "   commit: ${COMMIT_SHA:0:10}"

# 分支已存在（重跑场景）则强推更新；不存在则创建
if "$GH" api "repos/${FORK}/git/refs/heads/${BRANCH}" > /dev/null 2>&1; then
  "$GH" api -X PATCH "repos/${FORK}/git/refs/heads/${BRANCH}" -F sha="$COMMIT_SHA" -F force=true --jq '.ref'
else
  "$GH" api -X POST "repos/${FORK}/git/refs" -f ref="refs/heads/${BRANCH}" -f sha="$COMMIT_SHA" --jq '.ref'
fi

echo "== 开 PR 到 ${UPSTREAM} =="
PR_BODY="## 概要

- 包：https://github.com/${OWNER}/zp-rdm
- Release：https://github.com/${OWNER}/zp-rdm/releases/tag/${TAG}
- 安装包：NSIS（nullsoft），x64，user scope，静默参数 /S
- InstallerSha256 与 GitHub Release 资产 digest 校验一致

🤖 Generated with [Claude Code](https://claude.com/claude-code)"

if ! "$GH" api -X POST "repos/${UPSTREAM}/pulls" \
    -f title="$COMMIT_MSG" -f head="${OWNER}:${BRANCH}" -f base="master" \
    -f body="$PR_BODY" --jq '.html_url' 2> "$WORK/pr_err"; then
  # 422 多半是 PR 已存在，查出来报告
  if grep -qi "already exists" "$WORK/pr_err"; then
    "$GH" api "repos/${UPSTREAM}/pulls?head=${OWNER}:${BRANCH}&state=open" --jq '.[0].html_url'
  else
    cat "$WORK/pr_err"; exit 1
  fi
fi

echo "== 完成 =="
echo "提示：首次向 microsoft 仓库贡献，CLA bot 会在 PR 下留言，"
echo "届时需本人评论：@microsoft-github-policy-service agree"
echo "（github.com 被墙可用 gh 评论：gh pr comment <PR号> --repo ${UPSTREAM} --body \"@microsoft-github-policy-service agree\"）"
