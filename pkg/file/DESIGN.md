# File Package Design

## 背景

`pkg/file` 是 fox 项目里的独立文件与目录管理包。它提供一套简洁、安全的文件系统操作 API，覆盖日常开发中常见的目录创建、文件读写、路径处理、文件遍历、文件监听等场景。

第一版聚焦基础能力：路径操作、目录管理、文件读写、文件信息查询、递归遍历和文件监听。不和 `core/server`、`core/config` 等模块绑定，不负责配置热加载、日志文件管理等上层业务。业务层或上层包可以在后续基于 `pkg/file` 构建更高层的功能，但那不属于本包第一阶段范围。

## 目标

- 提供安全的目录操作：创建、删除、判断是否存在、确保目录存在（MkdirAll 幂等）。
- 提供安全的文件操作：读取、写入、追加、复制、移动、删除、判断是否存在。
- 提供路径处理工具：路径清理、扩展名提取、文件名解析、相对路径计算、路径拼接。
- 提供目录递归遍历：支持 glob 模式过滤、深度控制、文件/目录分离。
- 提供基于 fsnotify 的文件监听：监听文件和目录的创建、修改、删除、重命名事件。
- 提供临时文件和临时目录管理：创建、自动清理。
- 提供文件锁：基于 flock 的进程级文件锁。
- 所有操作返回明确错误，不使用 panic。

## 非目标

- 不提供文件内容解析（JSON、YAML、TOML 等由 `core/config` 或业务层处理）。
- 不提供文件压缩/解压缩。
- 不提供网络文件系统操作（FTP、S3 等）。
- 不提供文件加密/解密。
- 不提供文件内容搜索（grep 语义）。
- 不提供文件权限的细粒度管理（仅支持基础的 chmod 操作）。
- 不和 `core/config` 的配置热加载绑定，文件监听只提供原生 fsnotify 事件。
- 不提供文件同步、diff 等版本控制语义。

## 目录结构

```text
pkg/file/
  DESIGN.md           # 设计文档
  file.go             # 文件操作：读取、写入、追加、复制、移动、删除、存在判断
  dir.go              # 目录操作：创建、删除、确保存在、遍历、列表
  path.go             # 路径工具：清理、扩展名、文件名、相对路径、拼接
  info.go             # 文件信息：大小、修改时间、类型判断
  temp.go             # 临时文件和临时目录管理
  lock.go             # 文件锁（flock）
  watch.go            # 文件监听（fsnotify 封装）
  glob.go             # Glob 模式匹配和过滤
  option.go           # 通用选项类型
  file_test.go        # 文件操作测试
  dir_test.go         # 目录操作测试
  path_test.go        # 路径工具测试
  info_test.go        # 文件信息测试
  temp_test.go        # 临时文件测试
  lock_test.go        # 文件锁测试
  watch_test.go       # 文件监听测试
  glob_test.go        # Glob 测试
```

## 核心模型

第一版以标准库 `os`、`path/filepath`、`io`、`io/fs` 为基础，不引入额外依赖（除 fsnotify，项目已有）。所有公开函数保持无状态，不持有全局可变状态。

### 文件信息

```go
// FileInfo 文件信息的扩展封装。
type FileInfo struct {
    Name    string      // 文件名（不含路径）
    Path    string      // 完整路径
    Size    int64       // 文件大小（字节）
    Mode    fs.FileMode // 文件权限
    ModTime time.Time   // 最后修改时间
    IsDir   bool        // 是否为目录
}

// FromStdFileInfo 从标准库 fs.FileInfo 构建。
func FromStdFileInfo(fi fs.FileInfo, dir string) *FileInfo

// Type 返回文件类型描述：file、dir、symlink、other。
func (fi *FileInfo) Type() string

// Extension 返回文件扩展名（含点号），例如 ".go"。
func (fi *FileInfo) Extension() string

// Stem 返回不含扩展名的文件名。
func (fi *FileInfo) Stem() string
```

### 遍历条目

```go
// WalkEntry 遍历目录时的单个条目。
type WalkEntry struct {
    Path     string      // 相对于根目录的路径
    AbsPath  string      // 绝对路径
    Info     *FileInfo   // 文件信息
    Depth    int         // 相对根目录的深度（根目录为 0）
    Error    error       // 遍历此条目时遇到的错误（例如权限不足）
}

// WalkFunc 遍历回调函数。返回 error 时终止遍历；返回 SkipDir 跳过当前目录。
type WalkFunc func(entry *WalkEntry) error

// SkipDir 是 WalkFunc 返回的特殊错误，用于跳过当前目录。
var SkipDir = errors.New("file: skip directory")
```

### 监听事件

```go
// Event 文件系统事件。
type Event struct {
    Path      string    // 事件关联的文件路径
    Op        Op        // 事件类型
    Timestamp time.Time // 事件发生时间
}

// Op 事件类型，支持位组合。
type Op uint32

const (
    Create  Op = 1 << iota // 文件或目录创建
    Write                  // 文件写入
    Remove                 // 文件或目录删除
    Rename                 // 文件或目录重命名
    Chmod                  // 权限变更
)

// Watcher 文件监听器。
type Watcher struct {
    // 内部持有 fsnotify.Watcher 和配置。
}
```

### 文件锁

```go
// FileLock 进程级文件锁。
type FileLock struct {
    // 内部持有文件描述符和锁状态。
}

// LockType 锁类型。
type LockType int

const (
    ReadLock  LockType = iota // 共享读锁
    WriteLock                 // 独占写锁
)
```

### Glob

```go
// GlobResult 单次 Glob 匹配结果。
type GlobResult struct {
    Path    string      // 匹配的相对路径
    AbsPath string      // 匹配的绝对路径
    Info    *FileInfo   // 文件信息
}
```

## API 设计

### 文件操作

基础文件操作，封装标准库并提供更安全的默认行为。

```go
// ReadFile 读取文件全部内容。
func ReadFile(path string) ([]byte, error)

// ReadString 读取文件内容为字符串。
func ReadString(path string) (string, error)

// ReadLines 按行读取文件内容，返回字符串切片。空行保留。
func ReadLines(path string) ([]string, error)

// WriteFile 写入文件，文件不存在时创建，存在时覆盖。自动创建父目录。
func WriteFile(path string, data []byte, opts ...WriteOption) error

// WriteString 写入字符串到文件。
func WriteString(path string, data string, opts ...WriteOption) error

// WriteLine 写入单行内容，自动追加换行符。
func WriteLine(path string, data string, opts ...WriteOption) error

// AppendFile 追加内容到文件末尾，文件不存在时创建。自动创建父目录。
func AppendFile(path string, data []byte, opts ...WriteOption) error

// AppendString 追加字符串到文件末尾。
func AppendString(path string, data string, opts ...WriteOption) error

// AppendLine 追加单行内容，自动追加换行符。
func AppendLine(path string, data string, opts ...WriteOption) error

// CopyFile 复制文件。目标父目录不存在时自动创建。
func CopyFile(src, dst string, opts ...CopyOption) error

// CopyDir 递归复制目录。目标不能等于源目录，也不能位于源目录内部。
func CopyDir(src, dst string, opts ...CopyOption) error

// MoveFile 移动文件（重命名）。跨文件系统时自动 fallback 到复制+删除。
func MoveFile(src, dst string) error

// MoveDir 移动目录。跨文件系统时自动 fallback 到递归复制+删除。
func MoveDir(src, dst string) error

// RemoveFile 删除文件。文件不存在时不报错。
func RemoveFile(path string) error

// RemoveDir 删除目录及其所有内容。
func RemoveDir(path string) error

// Exists 判断路径是否存在（文件或目录）。
func Exists(path string) bool

// IsFile 判断路径是否为文件（非目录）。
func IsFile(path string) bool

// IsDir 判断路径是否为目录。
func IsDir(path string) bool

// IsEmpty 判断文件或目录是否为空。目录为空表示不含任何文件或子目录。
func IsEmpty(path string) (bool, error)

// IsSymlink 判断路径是否为符号链接。
func IsSymlink(path string) bool
```

#### WriteOption

```go
type WriteOption func(*writeConfig)

type writeConfig struct {
    perm    fs.FileMode // 文件权限，默认 0644
    dirPerm fs.FileMode // 父目录权限，默认 0755
}

// WithPerm 设置文件写入权限。
func WithPerm(perm fs.FileMode) WriteOption

// WithDirPerm 设置自动创建的父目录权限。
func WithDirPerm(perm fs.FileMode) WriteOption
```

#### CopyOption

```go
type CopyOption func(*copyConfig)

type copyConfig struct {
    perm       fs.FileMode // 目标文件权限，默认与源文件一致
    dirPerm    fs.FileMode // 目标目录权限，默认 0755
    preserveMode bool      // 是否保留源文件权限，默认 true
    overwrite    bool      // 是否覆盖已存在的目标文件，默认 true
}

// WithCopyPerm 设置复制后的目标文件权限（覆盖源文件权限）。
func WithCopyPerm(perm fs.FileMode) CopyOption

// WithNoOverwrite 禁止覆盖已存在的目标文件。
func WithNoOverwrite() CopyOption
```

### 目录操作

```go
// Mkdir 创建单级目录。父目录不存在时返回错误。
func Mkdir(path string, opts ...DirOption) error

// MkdirAll 递归创建目录，已存在时幂等返回。类似 os.MkdirAll 但自动清理路径。
func MkdirAll(path string, opts ...DirOption) error

// EnsureDir 确保目录存在，不存在时创建。MkdirAll 的语义别名，更强调幂等性。
func EnsureDir(path string, opts ...DirOption) error

// RemoveDir 删除目录及其所有内容。目录不存在时不报错。
func RemoveDir(path string) error

// RemoveEmpty 删除空目录。目录非空时返回错误。
func RemoveEmpty(path string) error

// CleanDir 清空目录内容，保留目录本身。
func CleanDir(path string) error

// List 列出目录下的直接子项（不递归）。返回文件名列表。
func List(path string) ([]string, error)

// ListFiles 列出目录下的直接文件（不含子目录）。
func ListFiles(path string) ([]*FileInfo, error)

// ListDirs 列出目录下的直接子目录。
func ListDirs(path string) ([]*FileInfo, error)

// ListAll 列出目录下的所有直接子项（含文件和目录）。
func ListAll(path string) ([]*FileInfo, error)

// Size 计算文件或目录的总大小（目录递归统计，字节）。
func Size(path string) (int64, error)

// Count 统计目录下的文件数量（递归）。
func Count(path string) (int64, error)
```

#### DirOption

```go
type DirOption func(*dirConfig)

type dirConfig struct {
    perm fs.FileMode // 目录权限，默认 0755
}

// WithDirPermission 设置目录创建权限。
func WithDirPermission(perm fs.FileMode) DirOption
```

### 遍历

```go
// Walk 递归遍历目录，类似 filepath.WalkDir 但提供更丰富的上下文。
func Walk(root string, fn WalkFunc, opts ...WalkOption) error

// WalkFiles 只遍历文件，跳过目录。
func WalkFiles(root string, fn WalkFunc, opts ...WalkOption) error

// WalkDirs 只遍历目录。
func WalkDirs(root string, fn WalkFunc, opts ...WalkOption) error
```

#### WalkOption

```go
type WalkOption func(*walkConfig)

type walkConfig struct {
    maxDepth  int      // 最大遍历深度，0 表示不限制
    follow    bool     // 是否跟随符号链接，默认 false
    skipDirs  []string // 需要跳过的目录名
    skipFiles []string // 需要跳过的文件名
    patterns  []string // glob 匹配模式，只遍历匹配的文件
}

// WithMaxDepth 设置最大遍历深度。
func WithMaxDepth(depth int) WalkOption

// WithFollowSymlinks 设置是否跟随符号链接。
func WithFollowSymlinks() WalkOption

// WithSkipDirs 设置需要跳过的目录名列表。
func WithSkipDirs(dirs ...string) WalkOption

// WithSkipFiles 设置需要跳过的文件名列表。
func WithSkipFiles(files ...string) WalkOption

// WithPattern 设置 glob 匹配模式，只遍历匹配的文件。
func WithPattern(pattern string) WalkOption
```

### Glob 匹配

```go
// Glob 在指定目录下匹配 glob 模式，返回匹配的文件列表。
// 模式支持 ** 递归匹配子目录。
func Glob(root string, pattern string, opts ...GlobOption) ([]*GlobResult, error)

// Match 判断文件名是否匹配 glob 模式。
func Match(pattern, name string) (bool, error)
```

#### GlobOption

```go
type GlobOption func(*globConfig)

type globConfig struct {
    maxDepth int  // 最大搜索深度
    onlyFile bool // 只匹配文件
    onlyDir  bool // 只匹配目录
}

// WithGlobMaxDepth 设置 Glob 最大搜索深度。根目录深度为 0，直接子项深度为 1；
// 该限制对普通递归和 ** 递归都生效。
func WithGlobMaxDepth(depth int) GlobOption

// WithGlobFilesOnly 设置 Glob 只匹配文件。
func WithGlobFilesOnly() GlobOption

// WithGlobDirsOnly 设置 Glob 只匹配目录。
func WithGlobDirsOnly() GlobOption
```

### 路径工具

纯函数，不访问文件系统。

```go
// Clean 清理路径，去除多余分隔符和 . 和 .. 。
func Clean(path string) string

// Ext 返回文件扩展名（含点号），例如 ".go"。
func Ext(path string) string

// Basename 返回文件名（含扩展名）。
func Basename(path string) string

// Stem 返回文件名（不含扩展名）。
func Stem(path string) string

// Dir 返回父目录路径。
func Dir(path string) string

// Join 拼接路径片段。
func Join(elem ...string) string

// Abs 返回绝对路径。相对路径基于当前工作目录。
func Abs(path string) (string, error)

// Rel 返回从 base 到 target 的相对路径。
func Rel(base, target string) (string, error)

// Home 返回当前用户主目录。
func Home() (string, error)

// SystemTempDir 返回系统临时目录。
func SystemTempDir() string

// EnsureExt 确保文件路径具有指定扩展名，没有时追加。
func EnsureExt(path, ext string) string

// ChangeExt 更改文件路径的扩展名。
func ChangeExt(path, ext string) string
```

### 文件信息

```go
// Stat 获取文件信息。
func Stat(path string) (*FileInfo, error)

// LStat 获取文件信息（不跟随符号链接）。
func LStat(path string) (*FileInfo, error)

// FileSize 获取文件大小（字节）。
func FileSize(path string) (int64, error)

// Size 获取文件或目录大小（目录递归统计，字节）。
func Size(path string) (int64, error)

// ModTime 获取文件最后修改时间。
func ModTime(path string) (time.Time, error)

// Perm 获取文件权限。
func Perm(path string) (fs.FileMode, error)

// Checksum 计算文件的 hash 值。支持 md5、sha1、sha256。
func Checksum(path string, algo string) (string, error)

// Equal 判断两个文件内容是否相同。
func Equal(a, b string) (bool, error)
```

### 临时文件和目录

```go
// TempFile 在指定目录创建临时文件，返回文件句柄。
// 调用方负责关闭文件。
func TempFile(opts ...TempOption) (*os.File, error)

// TempFilePath 创建临时文件并立即关闭，返回文件路径。
func TempFilePath(opts ...TempOption) (string, error)

// TempDirPath 在指定目录创建临时目录，返回目录路径。
func TempDirPath(opts ...TempOption) (string, error)

// TempDirWithCleanup 创建临时目录，返回目录路径和清理函数。
func TempDirWithCleanup(opts ...TempOption) (string, func() error, error)

// TempFileWithCleanup 创建临时文件，返回文件路径和清理函数。文件在创建后立即关闭。
func TempFileWithCleanup(opts ...TempOption) (string, func() error, error)

// RemoveTemp 清理临时文件或目录。
func RemoveTemp(path string) error

// GlobTemp 匹配临时目录下的文件模式。
func GlobTemp(pattern string) ([]string, error)

// WithTempPrefix 设置临时文件/目录名前缀。
func WithTempPrefix(prefix string) TempOption

// WithTempDir 设置临时文件/目录的父目录。
func WithTempDir(dir string) TempOption

// WithTempSuffix 设置临时文件/目录名后缀。
func WithTempSuffix(suffix string) TempOption

// WithTempExt 设置临时文件扩展名。
func WithTempExt(ext string) TempOption
```

### 文件锁

```go
// NewFileLock 创建文件锁。锁文件的父目录会自动创建。
func NewFileLock(path string) (*FileLock, error)

// Acquire 获取锁。WriteLock 为独占锁，ReadLock 为共享锁。
func (fl *FileLock) Acquire(lockType LockType) error

// TryAcquire 尝试获取锁，获取失败时立即返回错误（不阻塞）。
func (fl *FileLock) TryAcquire(lockType LockType) error

// AcquireWithTimeout 在超时时间内尝试获取锁。
func (fl *FileLock) AcquireWithTimeout(lockType LockType, timeout time.Duration) error

// Release 释放锁。
func (fl *FileLock) Release() error

// Close 释放锁并关闭文件描述符。
func (fl *FileLock) Close() error
```

`Acquire` 默认阻塞直到获取成功或被中断。`TryAcquire` 适合非阻塞场景。`AcquireWithTimeout` 适合需要超时控制的场景。`Release` 和 `Close` 可重复调用。

`FileLock` 的方法本身是并发安全的；文件锁基于 `syscall.Flock`，锁语义主要用于进程级协调。同一进程内需要 goroutine 互斥时，应使用 `sync.Mutex` 等内存锁。

### 文件监听

```go
// NewWatcher 创建文件监听器。
func NewWatcher(opts ...WatchOption) (*Watcher, error)

// Add 添加监听路径（文件或目录）。监听目录时，目录内文件的事件也会被捕获。
// Watcher 关闭后调用 Add 会返回错误。
func (w *Watcher) Add(path string) error

// Remove 移除监听路径。
// Watcher 关闭后调用 Remove 会返回错误。
func (w *Watcher) Remove(path string) error

// Events 返回事件通道。
func (w *Watcher) Events() <-chan Event

// Errors 返回错误通道。
func (w *Watcher) Errors() <-chan error

// Close 关闭监听器，释放所有资源。重复调用是安全的。
func (w *Watcher) Close() error

// Watch 是便捷函数，监听单个路径并在事件发生时调用回调。
func Watch(path string, fn func(Event), opts ...WatchOption) (stop func() error, err error)
```

#### WatchOption

```go
type WatchOption func(*watchConfig)

type watchConfig struct {
    recursive  bool          // 是否递归监听子目录
    debounce   time.Duration // 防抖时间，默认 0（不防抖）
    filter     Op            // 只接收指定类型的事件
    bufferSize int           // 事件缓冲区大小，默认 64
}

// WithRecursive 设置递归监听子目录。
func WithRecursive() WatchOption

// WithDebounce 设置事件防抖时间。
func WithDebounce(d time.Duration) WatchOption

// WithFilter 设置只接收指定类型的事件。
func WithFilter(op Op) WatchOption

// WithBufferSize 设置事件缓冲区大小。
func WithBufferSize(n int) WatchOption
```

防抖机制：

- 防抖窗口内同一文件的多次 Write 事件合并为一次回调。
- 防抖使用文件路径作为 key，不同文件独立防抖。
- 防抖仅适用于 Write 事件，Create、Remove、Rename、Chmod 不防抖。
- 防抖实现基于 `time.Timer`，`Close()` 时清理所有 pending timer。

递归监听：

- `WithRecursive()` 开启后，监听器会对已有子目录添加监听，并自动监听新创建的子目录。
- 递归监听在大目录下可能有性能问题，调用方应谨慎使用。
- 递归监听依赖底层 fsnotify，不同操作系统对符号链接、重命名、删除等事件的表现可能不同。

## 错误处理

所有公开函数返回 `error`，不使用 panic。错误类型：

```go
// PathError 路径相关错误。
type PathError struct {
    Op   string // 操作名称
    Path string // 相关路径
    Err  error  // 底层错误
}

func (e *PathError) Error() string
func (e *PathError) Unwrap() error

// IsNotExist 判断错误是否为"路径不存在"。
func IsNotExist(err error) bool

// IsExist 判断错误是否为"路径已存在"。
func IsExist(err error) bool

// IsPermission 判断错误是否为"权限不足"。
func IsPermission(err error) bool
```

错误语义：

- `RemoveFile` 删除不存在的文件不报错，保持幂等性。
- `RemoveDir` 删除不存在的目录不报错。
- `MkdirAll` 目录已存在不报错。
- `CopyFile` 目标已存在且 `WithNoOverwrite` 时返回 `*PathError`，`Err` 为 `fs.ErrExist`。
- `CopyDir` 目标等于源目录或位于源目录内部时返回 `*PathError`。
- `MoveFile` / `MoveDir` 跨文件系统时 fallback 到复制+删除；复制成功但删除源失败时返回 `*PathError`。
- `WriteFile` 自动创建父目录失败时返回 `*PathError`，包含父目录路径。

## 并发安全

- 所有公开函数无状态，天然并发安全。
- `Watcher` 内部通过 channel 传递事件，`Events()` 和 `Errors()` 返回的 channel 支持多消费者竞争读取（但同一事件只会被一个消费者接收）。
- `Watcher.Close()` 可重复调用，关闭后调用 `Add` / `Remove` 会返回错误。
- `FileLock` 的方法是 goroutine 安全的，但底层 `flock` 是进程级文件锁，不等同于同一进程内的内存互斥锁。

## 性能目标

第一版性能目标用于指导实现和测试，不作为严格 SLA：

- `ReadFile` 读取 100MB 文件应在 500ms 内完成。
- `WriteFile` 写入 100MB 文件（含父目录创建）应在 1 秒内完成。
- `CopyFile` 复制 100MB 文件应在 1 秒内完成。
- `Walk` 遍历 10 万个文件的目录应在 2 秒内完成。
- `Glob` 在 10 万个文件的目录中匹配模式应在 1 秒内完成。
- `List` 列出 1 万个文件的目录应在 100ms 内完成。
- 文件锁获取/释放应在 100μs 内完成。
- Watcher 事件从文件系统变更到回调触发的延迟应在 100ms 内。

## 示例

### 基础文件读写

```go
// 写入文件（自动创建父目录）
err := file.WriteString("/tmp/app/config.yaml", "key: value",
    file.WithPerm(0644),
)

// 读取文件
data, err := file.ReadString("/tmp/app/config.yaml")

// 按行读取
lines, err := file.ReadLines("/tmp/app/data.txt")

// 追加日志
err := file.AppendLine("/tmp/app/logs/app.log", "new log entry")
```

### 目录操作

```go
// 确保目录存在
err := file.EnsureDir("/tmp/app/data")

// 列出目录下的文件
files, err := file.ListFiles("/tmp/app/data")

// 递归遍历
err := file.Walk("/tmp/app", func(entry *file.WalkEntry) error {
    if entry.Info.IsDir {
        return nil
    }
    fmt.Printf("[%d] %s (%d bytes)\n", entry.Depth, entry.Path, entry.Info.Size)
    return nil
}, file.WithMaxDepth(3), file.WithPattern("*.go"))

// Glob 匹配
results, err := file.Glob("/tmp/app", "**/*.go")
for _, r := range results {
    fmt.Println(r.AbsPath)
}

// 计算目录大小
size, err := file.Size("/tmp/app")
```

### 文件复制和移动

```go
// 复制文件
err := file.CopyFile("/tmp/source.txt", "/tmp/dest.txt")

// 递归复制目录
err := file.CopyDir("/tmp/project", "/tmp/project-backup")

// 移动文件（跨文件系统自动 fallback）
err := file.MoveFile("/tmp/old.txt", "/tmp/new.txt")
```

### 文件监听

```go
// 监听单个文件
stop, err := file.Watch("/tmp/app/config.yaml", func(e file.Event) {
    fmt.Printf("config changed: %s %s\n", e.Op, e.Path)
})
defer stop()

// 监听目录（带防抖和递归）
w, err := file.NewWatcher(
    file.WithRecursive(),
    file.WithDebounce(500*time.Millisecond),
    file.WithFilter(file.Write|file.Create|file.Remove),
)
defer w.Close()

w.Add("/tmp/app/src")

for {
    select {
    case event := <-w.Events():
        fmt.Printf("event: %s %s\n", event.Op, event.Path)
    case err := <-w.Errors():
        fmt.Printf("error: %v\n", err)
    }
}
```

### 文件锁

```go
fl, err := file.NewFileLock("/tmp/app.lock")
if err != nil {
    log.Fatal(err)
}
defer fl.Close()

if err := fl.Acquire(file.WriteLock); err != nil {
    log.Fatal(err)
}
defer fl.Release()

// 独占操作
doSomething()
```

### 路径工具

```go
ext := file.Ext("config.yaml")         // ".yaml"
name := file.Stem("config.yaml")        // "config"
base := file.Basename("/tmp/config.yaml") // "config.yaml"

path := file.EnsureExt("config", ".yaml") // "config.yaml"
path = file.ChangeExt("data.json", ".yaml") // "data.yaml"
```

## 测试策略

- 文件操作测试覆盖：读写基础文件、长行、空文件、追加、复制、目录复制保护、移动和删除语义。
- 目录操作测试覆盖：创建单级/多级目录、删除空/非空目录、列表、文件数量和目录大小计算。
- 路径工具测试覆盖：各种路径格式、边界情况（空字符串、根路径、多斜杠、点号和双点号）。
- 文件信息测试覆盖：普通文件、目录、符号链接、不存在的路径。
- 临时文件测试覆盖：创建和自动清理、自定义前缀/后缀/扩展名。
- 文件锁测试覆盖：读锁共享、写锁独占、非阻塞获取、重复释放、自动创建锁目录和并发关闭。
- 文件监听测试覆盖：创建事件、关闭、关闭后的 Add/Remove、防抖、filter 和便捷 Watch。
- 错误处理测试覆盖：路径不存在、目录/文件类型不匹配、已存在、无效参数等可稳定模拟的错误。
- 并发安全测试覆盖：`FileLock` 生命周期方法的并发调用。

## 实施阶段

### Phase 1: 路径工具和文件操作

- 实现 `path.go`：Clean、Ext、Basename、Stem、Dir、Join、Abs、Rel、Home、SystemTempDir、EnsureExt、ChangeExt。
- 实现 `file.go`：ReadFile、ReadString、ReadLines、WriteFile、WriteString、WriteLine、AppendFile、AppendString、AppendLine、CopyFile、CopyDir、MoveFile、MoveDir、RemoveFile、RemoveDir、Exists、IsFile、IsDir、IsEmpty、IsSymlink。
- 实现 `info.go`：Stat、LStat、FileSize、ModTime、Perm、Checksum、Equal。
- 实现 `option.go`：WriteOption、CopyOption。
- 实现错误类型：PathError、IsNotExist、IsExist、IsPermission。

交付物：

- `path.go`
- `file.go`
- `info.go`
- `option.go`
- `path_test.go`
- `file_test.go`
- `info_test.go`

验收标准：

- 所有路径工具函数正确处理边界情况。
- 文件读写覆盖文本和二进制文件。
- CopyFile 和 MoveFile 正确处理跨文件系统场景。
- 自动创建父目录功能正常。
- 错误类型正确包装底层错误。
- Phase 1 相关单元测试通过。

### Phase 2: 目录操作和遍历

- 实现 `dir.go`：Mkdir、MkdirAll、EnsureDir、RemoveDir、RemoveEmpty、CleanDir、List、ListFiles、ListDirs、ListAll、Size、Count。
- 实现 `walk.go` 或在 `dir.go` 中实现：Walk、WalkFiles、WalkDirs、WalkFunc、WalkEntry、SkipDir。
- 实现 `glob.go`：Glob、Match。
- 实现 DirOption、WalkOption、GlobOption。

交付物：

- `dir.go`
- `glob.go`
- `dir_test.go`
- `glob_test.go`

验收标准：

- MkdirAll 幂等性正确。
- Walk 支持深度控制、glob 过滤、目录跳过。
- Glob 支持 ** 递归匹配。
- 大目录遍历性能达标。
- Phase 2 相关单元测试通过。

### Phase 3: 临时文件、文件锁和文件监听

- 实现 `temp.go`：TempFile、TempFilePath、TempDirPath、TempDirWithCleanup、TempFileWithCleanup、RemoveTemp、GlobTemp、TempOption。
- 实现 `lock.go`：NewFileLock、Acquire、TryAcquire、AcquireWithTimeout、Release、Close。
- 实现 `watch.go`：NewWatcher、Add、Remove、Events、Errors、Close、Watch、WatchOption。
- 实现 FileInfo 的 FromStdFileInfo、Type、Extension、Stem 方法。

交付物：

- `temp.go`
- `lock.go`
- `watch.go`
- `temp_test.go`
- `lock_test.go`
- `watch_test.go`

验收标准：

- 临时文件/目录创建和清理正常。
- 文件锁支持读锁共享和写锁独占。
- 文件锁超时控制正确。
- Watcher 正确捕获文件系统事件。
- Watcher 防抖机制正常工作。
- Watcher 递归监听能够添加已有子目录并监听新建子目录。
- Phase 3 相关单元测试通过。

## 风险和约束

- `CopyDir` 递归复制大量小文件时可能较慢，第一版不实现并行复制。
- `MoveDir` 跨文件系统 fallback 使用递归复制+删除，大目录移动时可能较慢且非原子操作。
- `Watcher` 基于 fsnotify，不同操作系统的行为可能有差异（例如 Linux 的 inotify 和 macOS 的 kqueue）。
- `Watcher` 递归监听在大目录下可能消耗较多资源，需要在文档中提醒。
- `FileLock` 基于 flock，仅在同一主机上有效，不支持分布式锁。
- `FileLock` 在 NFS 等网络文件系统上的行为可能不符合预期。
- `Checksum` 对大文件使用流式计算，不会将整个文件读入内存。
- `Walk` 不保证遍历顺序，需要排序时由调用方处理。
- 第一版不支持 Windows 特有的路径语义（盘符、UNC 路径），仅保证 POSIX 兼容。
