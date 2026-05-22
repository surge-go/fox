# 提交前验证协议

## 目标

在执行 commit 之前运行构建和测试，确保变更不会引入编译错误或测试失败。**没有验证不说完成。**

## 语言检测

根据项目文件自动检测语言和构建工具：

| 检测信号 | 语言 | 验证命令 |
|---------|------|---------|
| `go.mod` | Go | `go build ./...` → `go test ./...` → `go vet ./...` |
| `package.json` | Node.js | `npm run build` → `npm test` |
| `Makefile` | 通用 | `make build` → `make test` |
| `Cargo.toml` | Rust | `cargo build` → `cargo test` |
| `pom.xml` | Java | `mvn compile` → `mvn test` |

如果无法确定，询问用户使用什么验证命令。

## 执行顺序

### Step 1: 敏感信息扫描

```bash
# 检查暂存区是否包含敏感文件
git diff --staged --name-only | grep -iE '\.env|\.secret|\.key|\.pem|credential|\.p12|\.pfx'
```

如果命中，**立即警告并中止**。

### Step 2: 构建（Build）

运行项目构建命令。构建失败 = 不可提交。

```bash
# Go 项目示例
go build ./...

# 如果有 Makefile
make build
```

### Step 3: 测试（Test）

运行项目测试。测试失败需要用户确认是否仍要提交。

```bash
# Go 项目示例
go test ./...

# 如果有 Makefile
make test
```

### Step 4: Lint（可选）

如果项目配置了 lint 工具，运行检查：

```bash
# Go
golangci-lint run ./...

# Node
npm run lint
```

Lint 警告不阻塞提交，但应提醒用户。

## 验证结果处理

| 验证结果 | 处理 |
|---------|------|
| 全部通过 | 展示绿色 ✅，执行提交 |
| 构建失败 | 红色 ❌，中止提交，展示错误输出 |
| 测试失败 | 黄色 ⚠️，展示失败用例，询问用户是否继续 |
| Lint 警告 | 黄色 ⚠️，展示警告，不阻塞 |

## 输出格式

```
🔍 提交前验证
  ✅ 构建: 通过 (2.3s)
  ✅ 测试: 通过 — 42/42 (5.1s)
  ⚠️  Lint: 3 个警告 (1.2s)
     - auth/token.go:42: ineffectual assignment
     - api/handler.go:15: exported function lacks comment
     - service/user.go:88: unused parameter 'ctx'
  ✅ 敏感信息: 未发现

  是否继续提交？(y/n)
```

## 特殊情况

### `--no-verify` 参数

用户可使用 `--no-verify` 跳过验证，但需要 **二次确认**：

```
⚠️  你选择了跳过验证 (--no-verify)。
这可能导致提交包含编译错误或失败的测试。
确认跳过？(y/n)
```

### `--amend` 参数

修正上次提交时：
- 如果上次提交已 push，**强烈警告**（会改写远程历史）
- 运行相同的验证流程
- 展示将要修正的内容对比

### 无暂存文件

如果 `git diff --staged` 为空：
- 提示用户是否要 `git add` 全部/部分文件
- 展示未暂存文件列表供选择
- 不自动执行 `git add`
