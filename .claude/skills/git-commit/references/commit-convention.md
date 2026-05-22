# 中文提交消息规范

## Conventional Commits 格式（中文版）

```
<type>(<scope>): <中文subject>

<中文body>

<footer>
```

**核心原则：type 和 scope 保持英文，subject 和 body 使用中文。**

## 各字段规则

### type（必填，英文）

从以下选择最匹配的一个：

| type | 说明 | 示例 |
|------|------|------|
| `feat` | 新功能 | feat(user): 新增头像上传接口 |
| `fix` | 修复 bug | fix(auth): 修复 token 过期校验逻辑 |
| `refactor` | 重构（不改行为） | refactor(repo): 提取提交解析器 |
| `perf` | 性能优化 | perf(query): 批量查询替代循环单查 |
| `docs` | 文档 | docs(api): 更新接口文档 |
| `style` | 格式/风格 | style(lint): 修复 golangci-lint 警告 |
| `test` | 测试 | test(auth): 补充 token 过期单测 |
| `build` | 构建系统 | build(docker): 升级到 Go 1.22 |
| `ci` | CI 配置 | ci(github): 新增 lint 工作流 |
| `chore` | 杂项 | chore(deps): 升级 golang.org/x/crypto |
| `revert` | 回滚 | revert: 回滚 feat(user): 新增头像上传 |

### scope（推荐，英文）

表示影响范围，通常为模块名/包名：

- Go 项目：package 名称，如 `auth`、`user`、`handler`
- 多模块项目：`module/submodule`
- 跨模块变更：省略 scope

### subject（必填，中文）

- **不超过 50 个字符**
- 使用中文描述"做了什么"，**必须具体说明变更内容**
- 简洁精准，不说"怎么做的"
- 不要以句号结尾
- **禁止使用模糊词汇**：更新、修改、优化、调整（除非后面跟具体内容）

**好例子**：
- `feat(user): 新增头像上传接口`（具体说明新增了什么）
- `fix(auth): 修复 token 过期校验逻辑`（具体说明修复了什么）
- `refactor: 提取提交消息解析器`（具体说明提取了什么）
- `perf(query): 批量查询替代循环单查`（具体说明优化方式）
- `test(server): 新增 Context 绑定和验证测试`（具体说明测试内容）
- `feat(server): 新增 Recovery 中间件和 panic 处理`（具体说明新增功能）

**坏例子**：
- `feat(user): 添加了一个用户头像上传的功能接口`（太啰嗦）
- `fix: 修复 bug`（没有信息量）
- `update code`（中英混杂，模糊）
- `更新 server 包`（太模糊，没说更新了什么）
- `优化代码`（太模糊，没说优化了什么）
- `调整配置`（太模糊，没说调整了什么）

### body（可选，中文）

- 用中文解释 **WHY** — 为什么要做这个改动
- 不重复 WHAT — diff 和 subject 已经说明了
- 每行不超过 72 字符

**好例子**：
```
feat(order): 新增订单超时自动取消

原方案使用定时任务全表扫描，订单量大时延迟严重。
改为基于 Redis 延迟队列实现，延迟从分钟级降到秒级。
```

### footer（可选）

- 关联 issue：`Closes #123` 或 `Fixes #456`
- 破坏性变更：`BREAKING CHANGE: 中文描述`
- 多 issue：`Refs #123, #456`

## 消息生成流程

1. 读取变更分析结果（Phase 1 输出）
2. 读取近期 10 条 commit 历史，匹配项目风格
3. **详细分析 diff 内容**，识别具体变更：
   - 新增了哪些函数/结构体/接口/测试
   - 修复了什么具体问题
   - 重构了什么具体内容
   - 删除了什么功能
4. 推断 type → 推断 scope → 撰写 **具体的中文** subject
5. 判断是否需要中文 body（变更不明显时添加）
6. 检查是否有可关联的 issue
7. 输出给用户确认

**关键原则**：subject 必须让读者不看 diff 就能理解做了什么具体变更。

## 输出格式

```
📝 建议的提交信息：

feat(user): 新增头像上传接口

原方案仅支持通过 URL 设置头像，用户反馈体验差。
新增 multipart form 上传，图片自动压缩至 256x256。

Closes #42
```

## 用户确认

展示建议后等待用户确认：
- 用户说 "好" / "确认" / "提交" → 执行提交
- 用户修改内容 → 按修改后的消息提交
- 用户说 "拆分" → 进入 split 模式
- 用户说 "取消" → 中止
