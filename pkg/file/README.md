# pkg/file

文件与目录管理包，提供安全、简洁的文件系统操作 API。覆盖路径处理、文件读写、目录管理、递归遍历、Glob 匹配、文件监听、文件锁和临时文件管理等场景。基于标准库 `os`、`path/filepath`、`io/fs` 构建，仅额外依赖 `fsnotify`。

## 快速开始

```go
package main

import (
	"fmt"
	"log"

	"github.com/surge-go/fox/pkg/file"
)

func main() {
	// 写入文件（自动创建父目录）
	err := file.WriteString("/tmp/example/hello.txt", "你好，世界！")
	if err != nil {
		log.Fatal(err)
	}

	// 读取文件
	data, err := file.ReadString("/tmp/example/hello.txt")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(data)

	// 递归遍历目录
	err = file.Walk("/tmp/example", func(entry *file.WalkEntry) error {
		if entry.Info != nil && !entry.Info.IsDir {
			fmt.Printf("%s (%d bytes)\n", entry.AbsPath, entry.Info.Size)
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
}
```

## 核心功能

### 1. 文件读写

提供读取、写入、追加三组操作，写入和追加自动创建父目录。

```go
// 读取
data, err := file.ReadFile("config.yaml")
text, err := file.ReadString("config.yaml")
lines, err := file.ReadLines("data.txt")

// 写入（自动创建父目录，默认权限 0644）
err := file.WriteFile("output/data.txt", []byte("hello"))
err := file.WriteString("output/data.txt", "hello")
err := file.WriteLine("output/data.txt", "hello") // 自动追加换行符

// 追加
err := file.AppendFile("logs/app.log", []byte("new entry"))
err := file.AppendLine("logs/app.log", "new entry") // 自动追加换行符
```

可通过 `WriteOption` 控制权限：

```go
err := file.WriteFile("secret.key", data,
    file.WithPerm(0600),
    file.WithDirPerm(0700),
)
```

### 2. 文件复制与移动

复制支持权限保留和覆盖控制，移动支持跨文件系统自动 fallback。

```go
// 复制文件（默认保留源文件权限）
err := file.CopyFile("src.txt", "dst.txt")

// 复制时指定权限
err := file.CopyFile("src.txt", "dst.txt", file.WithCopyPerm(0644))

// 禁止覆盖已存在的目标
err := file.CopyFile("src.txt", "dst.txt", file.WithNoOverwrite())

// 递归复制目录
err := file.CopyDir("/project", "/backup")

// 移动文件（跨文件系统自动 fallback 到复制+删除）
err := file.MoveFile("old.txt", "new.txt")

// 移动目录
err := file.MoveDir("/old/path", "/new/path")
```

`CopyDir` 会拒绝将目录复制到自身或源目录内部，避免递归复制导致目录无限增长。

### 3. 目录管理

提供创建、删除、清理、列表等目录操作。

```go
// 创建目录
err := file.Mkdir("subdir")             // 单级，父目录不存在时报错
err := file.MkdirAll("a/b/c")           // 递归创建，已存在时幂等
err := file.EnsureDir("a/b/c")          // MkdirAll 的语义别名

// 删除
err := file.RemoveDir("/tmp/data")       // 递归删除，不存在时不报错
err := file.RemoveEmpty("empty_dir")     // 仅删除空目录
err := file.CleanDir("/tmp/cache")       // 清空内容，保留目录本身

// 列表
names, err := file.List("/tmp")                    // 文件名列表
files, err := file.ListFiles("/tmp")               // 仅文件
dirs, err := file.ListDirs("/tmp")                 // 仅子目录
all, err := file.ListAll("/tmp")                   // 全部子项

// 统计
size, err := file.Size("/tmp/data")                // 递归大小（字节）
count, err := file.Count("/tmp/data")              // 递归文件数
```

### 4. 路径工具

纯函数，不访问文件系统。

```go
file.Ext("config.yaml")         // ".yaml"
file.Stem("config.yaml")        // "config"
file.Basename("/tmp/config.yaml") // "config.yaml"
file.Dir("/tmp/config.yaml")    // "/tmp"
file.Clean("/tmp//a/../b")      // "/tmp/b"
file.Join("a", "b", "c")       // "a/b/c"

abs, _ := file.Abs("relative")  // 绝对路径
rel, _ := file.Rel("/a", "/a/b") // "b"
home, _ := file.Home()           // 用户主目录

file.EnsureExt("config", ".yaml")  // "config.yaml"
file.ChangeExt("data.json", ".yaml") // "data.yaml"
```

### 5. 文件信息

```go
info, err := file.Stat("main.go")
fmt.Println(info.Name, info.Size, info.Type())

// 不跟随符号链接
info, err := file.LStat("link")

// 工具函数
exists := file.Exists("path")
isFile := file.IsFile("path")
isDir := file.IsDir("path")
isEmpty, _ := file.IsEmpty("path")
isLink := file.IsSymlink("path")

// 计算文件 hash（支持 md5、sha1、sha256）
hash, err := file.Checksum("data.bin", "sha256")

// 比较两个文件内容是否相同
equal, err := file.Equal("a.txt", "b.txt")
```

### 6. 目录遍历

基于 `filepath.WalkDir`，提供深度控制、glob 过滤、目录跳过等能力。

```go
// 基础遍历
err := file.Walk("/project", func(entry *file.WalkEntry) error {
    fmt.Printf("[%d] %s\n", entry.Depth, entry.AbsPath)
    return nil
})

// 只遍历文件
err := file.WalkFiles("/project", func(entry *file.WalkEntry) error {
    fmt.Println(entry.AbsPath)
    return nil
})

// 带选项的遍历
err := file.Walk("/project", fn,
    file.WithMaxDepth(3),              // 最大深度
    file.WithSkipDirs("node_modules", ".git"),  // 跳过目录
    file.WithSkipFiles(".DS_Store"),   // 跳过文件
    file.WithPattern("*.go"),          // glob 过滤
)

// 跳过子目录
err := file.Walk("/project", func(entry *file.WalkEntry) error {
    if entry.Info.IsDir && entry.Info.Name == "vendor" {
        return file.SkipDir
    }
    return nil
})
```

### 7. Glob 匹配

支持 `**` 递归匹配子目录。

```go
// 基础匹配
results, err := file.Glob("/project", "*.go")

// 递归匹配
results, err := file.Glob("/project", "**/*.go")

// 带选项
results, err := file.Glob("/project", "**/*.go",
    file.WithGlobMaxDepth(3),    // 最大搜索深度，根目录深度为 0，直接子项深度为 1
    file.WithGlobFilesOnly(),    // 只匹配文件
    file.WithGlobDirsOnly(),     // 只匹配目录（与 FilesOnly 互斥）
)

for _, r := range results {
    fmt.Println(r.AbsPath, r.Info.Size)
}

// 单文件名匹配
matched, _ := file.Match("*.go", "main.go") // true
```

### 8. 文件监听

基于 fsnotify 封装，支持防抖、事件过滤、递归监听。

```go
// 便捷方式：监听单个路径
stop, err := file.Watch("config.yaml", func(e file.Event) {
    fmt.Printf("%s: %s\n", e.Op, e.Path)
})
defer stop()

// 完整控制
w, err := file.NewWatcher(
    file.WithRecursive(),                         // 递归监听子目录
    file.WithDebounce(500*time.Millisecond),      // Write 事件防抖
    file.WithFilter(file.Write|file.Create),      // 只接收指定事件
    file.WithBufferSize(128),                     // 事件缓冲区大小
)
defer w.Close()

w.Add("/project/src")

for {
    select {
    case event := <-w.Events():
        fmt.Printf("[%s] %s\n", event.Op, event.Path)
    case err := <-w.Errors():
        fmt.Printf("error: %v\n", err)
    }
}
```

`Watcher.Close` 可重复调用；关闭后继续调用 `Add` 或 `Remove` 会返回错误。

事件类型支持位组合：

| 事件 | 说明 |
|------|------|
| `file.Create` | 文件或目录创建 |
| `file.Write` | 文件写入 |
| `file.Remove` | 文件或目录删除 |
| `file.Rename` | 文件或目录重命名 |
| `file.Chmod` | 权限变更 |

### 9. 文件锁

基于 `syscall.Flock` 的进程级文件锁。支持共享读锁和独占写锁。

```go
fl, err := file.NewFileLock("/tmp/app.lock")
defer fl.Close()

// 阻塞获取
err = fl.Acquire(file.WriteLock)
defer fl.Release()

// 非阻塞尝试
err = fl.TryAcquire(file.WriteLock)

// 带超时
err = fl.AcquireWithTimeout(file.WriteLock, 5*time.Second)
```

### 10. 临时文件与目录

```go
// 创建临时文件（调用方负责关闭）
f, err := file.TempFile(
    file.WithTempPrefix("app-"),
    file.WithTempExt(".tmp"),
)
defer f.Close()

// 创建临时文件路径（自动关闭）
path, err := file.TempFilePath(file.WithTempPrefix("app-"))

// 创建临时目录
dirPath, err := file.TempDirPath(file.WithTempPrefix("app-"))

// 创建临时目录并返回清理函数
dirPath, cleanup, err := file.TempDirWithCleanup()
defer cleanup()

// 创建临时文件并返回清理函数
path, cleanup, err := file.TempFileWithCleanup()
defer cleanup()
```

## API 参考

### 错误处理

所有操作返回 `*PathError`，支持 `errors.Is` 和 `errors.As`：

```go
type PathError struct {
    Op   string // 操作名称
    Path string // 相关路径
    Err  error  // 底层错误
}
```

辅助函数：

| 函数 | 说明 |
|------|------|
| `file.IsNotExist(err)` | 路径不存在 |
| `file.IsExist(err)` | 路径已存在 |
| `file.IsPermission(err)` | 权限不足 |

错误语义：

- `RemoveFile` / `RemoveDir` 删除不存在的路径不报错（幂等）
- `MkdirAll` 目录已存在不报错（幂等）
- `CopyFile` 目标已存在且 `WithNoOverwrite` 时返回 `fs.ErrExist`
- `CopyDir` 目标等于源目录或位于源目录内部时返回错误

### WriteOption

| 选项 | 说明 | 默认值 |
|------|------|--------|
| `WithPerm(perm)` | 文件权限 | `0644` |
| `WithDirPerm(perm)` | 自动创建的父目录权限 | `0755` |

### CopyOption

| 选项 | 说明 | 默认值 |
|------|------|--------|
| `WithCopyPerm(perm)` | 目标文件权限（覆盖源文件权限） | 保留源文件权限 |
| `WithNoOverwrite()` | 禁止覆盖已存在的目标文件 | 允许覆盖 |

### DirOption

| 选项 | 说明 | 默认值 |
|------|------|--------|
| `WithDirPermission(perm)` | 目录创建权限 | `0755` |

### WalkOption

| 选项 | 说明 | 默认值 |
|------|------|--------|
| `WithMaxDepth(n)` | 最大遍历深度，0 表示不限制 | `0` |
| `WithFollowSymlinks()` | 跟随符号链接（自动检测循环） | 不跟随 |
| `WithSkipDirs(dirs...)` | 跳过指定目录名 | 无 |
| `WithSkipFiles(files...)` | 跳过指定文件名 | 无 |
| `WithPattern(pattern)` | glob 模式过滤（单层匹配） | 无 |

### GlobOption

| 选项 | 说明 | 默认值 |
|------|------|--------|
| `WithGlobMaxDepth(n)` | 最大搜索深度，根目录深度为 0，对 `**` 生效 | `0`（不限制） |
| `WithGlobFilesOnly()` | 只匹配文件 | 匹配文件和目录 |
| `WithGlobDirsOnly()` | 只匹配目录 | 匹配文件和目录 |

### WatchOption

| 选项 | 说明 | 默认值 |
|------|------|--------|
| `WithRecursive()` | 递归监听子目录 | 不递归 |
| `WithDebounce(d)` | Write 事件防抖时间 | `0`（不防抖） |
| `WithFilter(op)` | 只接收指定事件类型 | 接收全部事件 |
| `WithBufferSize(n)` | 事件缓冲区大小 | `64` |

### TempOption

| 选项 | 说明 | 默认值 |
|------|------|--------|
| `WithTempPrefix(prefix)` | 临时文件/目录名前缀 | `""` |
| `WithTempSuffix(suffix)` | 临时文件/目录名后缀 | `""` |
| `WithTempExt(ext)` | 临时文件扩展名 | `""` |
| `WithTempDir(dir)` | 临时文件/目录的父目录 | 系统临时目录 |

## 最佳实践

### 1. 文件删除幂等

`RemoveFile` 和 `RemoveDir` 在路径不存在时不报错，可安全重复调用：

```go
// 安全删除，无需先检查是否存在
if err := file.RemoveFile("temp.txt"); err != nil {
    log.Fatal(err)
}
```

### 2. 文件锁使用

`FileLock` 的方法本身是并发安全的，但锁语义来自 `syscall.Flock`，主要用于进程级协调；同一进程内需要 goroutine 互斥时仍应使用 `sync.Mutex` 等内存锁：

```go
fl, err := file.NewFileLock("/var/run/app.lock")
if err != nil {
    log.Fatal(err)
}
defer fl.Close()

if err := fl.AcquireWithTimeout(file.WriteLock, 10*time.Second); err != nil {
    log.Fatal("获取锁超时")
}
defer fl.Release()

// 执行独占操作
```

### 3. 大文件处理

`Checksum` 使用流式计算，不会将整个文件读入内存：

```go
hash, err := file.Checksum("large.iso", "sha256")
```

`ReadLines` 支持最长 1MB 的单行，适合大部分日志文件：

```go
lines, err := file.ReadLines("access.log")
```

### 4. 监听防抖

频繁修改的文件建议开启防抖，避免短时间内触发大量事件：

```go
w, err := file.NewWatcher(
    file.WithDebounce(300 * time.Millisecond),
    file.WithFilter(file.Write),
)
```

### 5. 跨文件系统移动

`MoveFile` 和 `MoveDir` 在遇到 `EXDEV`（跨文件系统）错误时自动 fallback 到复制+删除：

```go
// 无需关心是否跨文件系统
err := file.MoveFile("/mnt/a/file.txt", "/mnt/b/file.txt")
```

## 注意事项

1. `FileLock` 基于 `syscall.Flock`，仅在同一主机上有效，不支持分布式锁。
2. `FileLock` 在 NFS 等网络文件系统上的行为可能不符合预期。
3. `Watcher` 递归监听在大目录下可能消耗较多资源，建议谨慎使用。
4. `Watcher` 基于 fsnotify，不同操作系统的行为可能有差异（Linux inotify / macOS kqueue）。
5. `MoveDir` 跨文件系统 fallback 使用递归复制+删除，大目录移动时可能较慢且非原子操作。
6. `Walk` 不保证遍历顺序，需要排序时由调用方处理。
7. `WithFollowSymlinks` 通过 `(dev, inode)` 检测符号链接循环，避免无限递归。
8. 路径工具函数（`path.go`）为纯函数，不访问文件系统。
9. `WithGlobMaxDepth` 对普通递归和 `**` 递归都生效。

## 依赖

- `github.com/fsnotify/fsnotify` — 跨平台文件系统事件通知
- `os`、`path/filepath`、`io`、`io/fs`、`crypto/*` — 标准库

## 相关文档

- [设计文档](./DESIGN.md)
- [fsnotify 文档](https://github.com/fsnotify/fsnotify)
