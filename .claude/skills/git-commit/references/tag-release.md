# Tag + Release 发版协议

## 目标

基于提交历史自动推断版本号、生成中文 Release Notes、创建 Tag 和 GitHub Release。

## 语义化版本（SemVer）

格式：`MAJOR.MINOR.PATCH[-prerelease]`

| 版本级别 | 递增条件 | 示例 |
|---------|---------|------|
| MAJOR | 有 `BREAKING CHANGE` 或不兼容的 API 变更 | 1.0.0 → 2.0.0 |
| MINOR | 有 `feat` 类型提交 | 1.0.0 → 1.1.0 |
| PATCH | 仅有 `fix` / `perf` / `refactor` 等 | 1.0.0 → 1.0.1 |

预发布版本后缀：

| 类型 | 后缀 | 示例 |
|------|------|------|
| Alpha | `-alpha.N` | 1.1.0-alpha.1 |
| Beta | `-beta.N` | 1.1.0-beta.1 |
| RC | `-rc.N` | 1.1.0-rc.1 |

## 执行步骤

### Step 1: 收集版本信息

```bash
# 查看已有标签
git tag --sort=-v:refname | head -10

# 最新标签
git describe --tags --abbrev=0 2>/dev/null

# 自上个 tag 以来的所有提交
git log <last-tag>..HEAD --oneline

# 如果没有 tag，获取所有提交
git log --oneline
```

### Step 2: 推断版本号

1. 获取当前最新 tag（如无，默认从 `v0.1.0` 开始）
2. 分析自上个 tag 以来的 commit 类型：
   - 包含 `BREAKING CHANGE` 或 `feat!:` → 递增 MAJOR
   - 包含 `feat` → 递增 MINOR
   - 包含 `fix` / `perf` → 递增 PATCH
   - 仅有其他类型 → 递增 PATCH
3. 如果有 `--prerelease` 参数，追加预发布后缀

**版本推断示例**：

```
当前版本: v1.2.3
本次提交分析:
  - feat(user): 新增头像上传接口 ← MINOR
  - fix(auth): 修复 token 过期 ← PATCH
  - refactor: 提取工具函数 ← PATCH

推断结果: v1.3.0（有 feat，递增 MINOR）
```

### Step 3: 生成中文 Release Notes

根据 commit 列表自动分类整理：

```markdown
## v1.3.0 (2026-04-14)

### ✨ 新功能
- 新增头像上传接口 (feat/user)
- 新增批量导出功能 (feat/export)

### 🐛 Bug 修复
- 修复 token 过期校验逻辑 (fix/auth)
- 修复分页查询越界问题 (fix/query)

### ⚡ 性能优化
- 批量查询替代循环单查 (perf/query)

### 🔧 重构
- 提取工具函数到公共包 (refactor)

### 📦 构建
- 升级 Go 版本至 1.22 (build)

---

**完整变更**: v1.2.3...v1.3.0
**贡献者**: @user1, @user2
```

分类规则：

| type | 分类名 | 图标 |
|------|--------|------|
| `feat` | 新功能 | ✨ |
| `fix` | Bug 修复 | 🐛 |
| `perf` | 性能优化 | ⚡ |
| `refactor` | 重构 | 🔧 |
| `docs` | 文档 | 📝 |
| `test` | 测试 | ✅ |
| `build` | 构建 | 📦 |
| `ci` | CI/CD | 👷 |
| `chore` | 其他 | 🔄 |

### Step 4: 创建 Annotated Tag

```bash
# Annotated tag（带消息）
git tag -a v1.3.0 -m "v1.3.0

新功能:
- 新增头像上传接口

Bug 修复:
- 修复 token 过期校验逻辑
"
```

**必须使用 `-a` 创建 annotated tag**，不使用轻量标签。

### Step 5: 创建 GitHub Release

```bash
# 检查是否在 GitHub 仓库
git remote get-url origin

# 创建 Release
gh release create v1.3.0 \
  --title "v1.3.0" \
  --notes "## v1.3.0 (2026-04-14)

### ✨ 新功能
- 新增头像上传接口 (feat/user)

### 🐛 Bug 修复
- 修复 token 过期校验逻辑 (fix/auth)
" \
  --latest
```

预发布版本额外加 `--prerelease`：

```bash
gh release create v1.3.0-rc.1 \
  --title "v1.3.0-rc.1" \
  --notes "..." \
  --prerelease
```

### Step 6: 推送

```bash
# 推送 tag
git push origin v1.3.0

# Release 会在 gh release create 时自动关联远程 tag
```

## 输出格式

```
🏷️  版本发布

  当前版本: v1.2.3
  推断版本: v1.3.0
  依据: 2 个 feat, 1 个 fix, 1 个 refactor

📝 Release Notes:

  ## v1.3.0 (2026-04-14)
  ### ✨ 新功能
  - 新增头像上传接口 (feat/user)
  ### 🐛 Bug 修复
  - 修复 token 过期校验逻辑 (fix/auth)
  ...

  确认创建 tag + release？(y/n)
```

## 用户确认

- 用户确认版本号 → 创建 tag
- 用户修改版本号 → 按修改后的版本号执行
- 用户说 "只打 tag" → 只创建 tag，不发 release
- 用户说 "取消" → 中止

## 特殊情况

### 首次发版（无历史 tag）

- 建议从 `v0.1.0` 或 `v1.0.0` 开始
- 展示所有历史 commit 供确认
- 询问用户起始版本号

### Hotfix 发版

- 从 main/master 分支打 tag
- 版本号只递增 PATCH
- Release Notes 标注为 hotfix

### 未安装 gh CLI

- 只创建本地 tag
- 提示用户安装 gh CLI 以使用 release 功能
- 提供 GitHub 网页端创建 release 的链接
