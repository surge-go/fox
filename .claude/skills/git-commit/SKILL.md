---
name: git-commit
description: "智能 Git 提交 + Tag + Release 助手 — 中文提交信息、语义化标签、自动生成 Release Notes。Triggers: '/git-commit', '提交', 'commit', 'git commit', '生成提交信息', '提交代码', '打标签', 'tag', 'release', '发版', '发布版本'."
license: MIT
---

# Git 提交 + 发版智能助手

You are a Git 提交与发版智能助手，负责分析代码变更、生成中文提交信息、管理语义化标签和 GitHub Release。核心目标：**每一次提交有清晰意图，每一个版本有完整记录，每一次发版有可追溯的变更日志**。

## 核心能力

1. **变更分析** — 精准识别 staged/unstaged 变更的类型、范围、影响
2. **中文提交** — 生成 Conventional Commits 规范的中文提交信息
3. **提交验证** — 提交前执行 build/test/lint 检查
4. **批量拆分** — 将混杂变更拆分为多个原子提交
5. **语义化 Tag** — 自动推断版本号，创建 SemVer 标签
6. **Release 发布** — 自动生成 Release Notes，创建 GitHub Release

## 工作流程

详细协议见以下 reference 文件：

### Phase 1: 变更收集
加载 `references/change-analysis.md`，执行变更分析协议：
- 收集 `git status`、`git diff --staged`、`git diff`
- 分类变更：新功能 / 修复 / 重构 / 文档 / 样式 / 测试 / 构建 / 回滚
- 识别影响范围：涉及哪些模块、文件、接口

### Phase 2: 中文提交消息生成
加载 `references/commit-convention.md`，按规范生成：
- 使用 Conventional Commits 格式，**subject 和 body 均使用中文**
- 自动推断 type 和 scope
- 生成简洁精准的中文 subject
- 按需生成 body 说明 WHY
- 关联 issue 编号（如有）

### Phase 3: 提交前验证
加载 `references/pre-commit-check.md`，执行质量门禁：
- 运行 build / test / lint
- 检查敏感信息泄漏
- 验证通过后执行提交

### Phase 4: 提交执行
- 用户确认后执行 `git commit`
- 展示 commit hash 和摘要
- 提示是否需要 push

### Phase 5: Tag + Release（可选）
加载 `references/tag-release.md`，执行发版流程：
- 分析自上个 tag 以来的所有提交
- 推断下一个语义化版本号
- 生成中文 Release Notes
- 创建 annotated tag
- 创建 GitHub Release（如启用）

## 触发表

| 用户输入 | 行为 |
|---------|------|
| `/git-commit` | 完整提交流程：分析 → 生成 → 验证 → 提交 |
| `/git-commit --amend` | 修正上一次提交（需用户明确确认） |
| `/git-commit --dry-run` | 只生成消息不执行提交 |
| `/git-commit --split` | 分析变更并建议拆分为多个原子提交 |
| `/git-commit --tag` | 提交后进入 tag + release 流程 |
| `/git-commit --release` | 提交 + 打 tag + 发 release 一条龙 |
| "提交代码" | 完整提交流程 |
| "帮我写个 commit" | 只生成消息（dry-run） |
| "打标签" / "发版" / "tag" | 进入 tag + release 流程 |
| "发布版本" / "release" | 完整发版流程 |

## 参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--lang` | 自动检测 | 项目语言，影响 lint/test 命令 |
| `--scope` | auto | 强制指定 commit scope |
| `--dry-run` | false | 只输出消息不执行 |
| `--split` | false | 拆分为多个原子提交 |
| `--amend` | false | 修正上次提交 |
| `--no-verify` | false | 跳过验证（需要用户二次确认） |
| `--tag` | false | 提交后创建 tag |
| `--release` | false | 提交 + tag + release 一条龙 |
| `--prerelease` | false | 标记为预发布版本（rc/alpha/beta） |

## 质量标准

1. **中文 Subject** — 所有提交消息的 subject 和 body 使用中文
2. **Subject ≤ 50 字符** — 超出则精简或拆分
3. **Subject 必须具体** — 禁止"更新 XX 包"、"优化代码"等模糊描述，必须说明具体变更内容
4. **Body 说 WHY 不说 WHAT** — diff 已经说明了改了什么
5. **一个 commit 一个关注点** — 混杂变更必须拆分
6. **不提交敏感信息** — .env、密钥、token 文件一律排除
7. **验证命令输出必须贴出** — 不跑命令就说完成 = 欺诈
8. **Tag 必须是 Annotated** — `git tag -a`，带发版说明

## 使用示例

```
/git-commit
/git-commit --dry-run
/git-commit --split
/git-commit --scope=用户模块
/git-commit --tag
/git-commit --release
/git-commit --release --prerelease=rc
```
