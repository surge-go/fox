package file

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Event 文件系统事件。
type Event struct {
	Path      string    // 事件关联的文件路径
	Op        Op        // 事件类型
	Timestamp time.Time // 事件发生时间
}

// Op 事件类型，支持位组合。
type Op uint32

const (
	Create Op = 1 << iota // 文件或目录创建
	Write                 // 文件写入
	Remove                // 文件或目录删除
	Rename                // 文件或目录重命名
	Chmod                 // 权限变更
)

func (o Op) String() string {
	if o == 0 {
		return "unknown"
	}
	var parts []string
	if o&Create != 0 {
		parts = append(parts, "create")
	}
	if o&Write != 0 {
		parts = append(parts, "write")
	}
	if o&Remove != 0 {
		parts = append(parts, "remove")
	}
	if o&Rename != 0 {
		parts = append(parts, "rename")
	}
	if o&Chmod != 0 {
		parts = append(parts, "chmod")
	}
	if len(parts) == 0 {
		return "unknown"
	}
	return strings.Join(parts, "|")
}

// WatchOption 文件监听选项。
type WatchOption func(*watchConfig)

type watchConfig struct {
	recursive  bool
	debounce   time.Duration
	filter     Op
	bufferSize int
}

func defaultWatchConfig() *watchConfig {
	return &watchConfig{
		bufferSize: 64,
	}
}

// WithRecursive 设置递归监听子目录。
func WithRecursive() WatchOption {
	return func(c *watchConfig) {
		c.recursive = true
	}
}

// WithDebounce 设置事件防抖时间。
func WithDebounce(d time.Duration) WatchOption {
	return func(c *watchConfig) {
		c.debounce = d
	}
}

// WithFilter 设置只接收指定类型的事件。
func WithFilter(op Op) WatchOption {
	return func(c *watchConfig) {
		c.filter = op
	}
}

// WithBufferSize 设置事件缓冲区大小。
func WithBufferSize(n int) WatchOption {
	return func(c *watchConfig) {
		c.bufferSize = n
	}
}

// Watcher 文件监听器。
type Watcher struct {
	w      *fsnotify.Watcher
	events chan Event
	errors chan error
	config *watchConfig
	done   chan struct{}
	closed bool
	mu     sync.Mutex
	timers map[string]*time.Timer
	wg     sync.WaitGroup
}

// NewWatcher 创建文件监听器。
func NewWatcher(opts ...WatchOption) (*Watcher, error) {
	cfg := defaultWatchConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, &PathError{Op: "watch", Err: err}
	}

	w := &Watcher{
		w:      fw,
		events: make(chan Event, cfg.bufferSize),
		errors: make(chan error, cfg.bufferSize),
		config: cfg,
		done:   make(chan struct{}),
		timers: make(map[string]*time.Timer),
	}

	w.wg.Add(1)
	go w.loop()
	return w, nil
}

// Add 添加监听路径（文件或目录）。
func (w *Watcher) Add(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return &PathError{Op: "watch", Path: path, Err: err}
	}

	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return &PathError{Op: "watch", Path: absPath, Err: errors.New("watcher is closed")}
	}
	w.mu.Unlock()

	if err := w.w.Add(absPath); err != nil {
		return &PathError{Op: "watch", Path: absPath, Err: err}
	}

	if w.config.recursive {
		if err := w.addRecursive(absPath); err != nil {
			return err
		}
	}
	return nil
}

// Remove 移除监听路径。
func (w *Watcher) Remove(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return &PathError{Op: "watch", Path: path, Err: err}
	}

	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return &PathError{Op: "watch", Path: absPath, Err: errors.New("watcher is closed")}
	}
	w.mu.Unlock()

	if err := w.w.Remove(absPath); err != nil {
		return &PathError{Op: "watch", Path: absPath, Err: err}
	}
	return nil
}

// Events 返回事件通道。
func (w *Watcher) Events() <-chan Event {
	return w.events
}

// Errors 返回错误通道。
func (w *Watcher) Errors() <-chan error {
	return w.errors
}

// Close 关闭监听器，释放所有资源。
func (w *Watcher) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	close(w.done)
	for _, t := range w.timers {
		t.Stop()
	}
	w.timers = nil
	w.mu.Unlock()

	// Close fsnotify watcher to unblock the loop goroutine from kevent/inotify.
	err := w.w.Close()

	// Wait for the loop goroutine to exit before closing channels.
	w.wg.Wait()

	close(w.events)
	close(w.errors)
	if err != nil {
		return &PathError{Op: "watch", Err: err}
	}
	return nil
}

func (w *Watcher) loop() {
	defer w.wg.Done()
	for {
		select {
		case <-w.done:
			return
		case e, ok := <-w.w.Events:
			if !ok {
				return
			}
			op := convertOp(e.Op)
			if w.config.filter != 0 && op&w.config.filter == 0 {
				continue
			}

			// Recursive: auto-add new directories
			if w.config.recursive && op == Create {
				if info, err := os.Stat(e.Name); err == nil && info.IsDir() {
					w.addRecursive(e.Name)
				}
			}

			if w.config.debounce > 0 && op == Write {
				w.debounceEvent(e.Name, op)
				continue
			}

			w.sendEvent(e.Name, op)
		case err, ok := <-w.w.Errors:
			if !ok {
				return
			}
			select {
			case w.errors <- err:
			case <-w.done:
				return
			}
		}
	}
}

func (w *Watcher) debounceEvent(path string, op Op) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Don't create new timers after Close has been called.
	if w.closed {
		return
	}

	if t, ok := w.timers[path]; ok {
		t.Stop()
	}

	w.timers[path] = time.AfterFunc(w.config.debounce, func() {
		w.mu.Lock()
		if w.closed {
			w.mu.Unlock()
			return
		}
		delete(w.timers, path)
		w.mu.Unlock()
		w.sendEvent(path, op)
	})
}

func (w *Watcher) sendEvent(path string, op Op) {
	event := Event{
		Path:      path,
		Op:        op,
		Timestamp: time.Now(),
	}
	select {
	case w.events <- event:
	case <-w.done:
	}
}

func (w *Watcher) addRecursive(dir string) error {
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible entries
		}
		if d.IsDir() {
			return w.w.Add(path)
		}
		return nil
	})
}

func convertOp(op fsnotify.Op) Op {
	var result Op
	if op&fsnotify.Create != 0 {
		result |= Create
	}
	if op&fsnotify.Write != 0 {
		result |= Write
	}
	if op&fsnotify.Remove != 0 {
		result |= Remove
	}
	if op&fsnotify.Rename != 0 {
		result |= Rename
	}
	if op&fsnotify.Chmod != 0 {
		result |= Chmod
	}
	return result
}

// Watch 是便捷函数，监听单个路径并在事件发生时调用回调。返回 stop 函数用于停止监听。
func Watch(path string, fn func(Event), opts ...WatchOption) (stop func() error, err error) {
	w, err := NewWatcher(opts...)
	if err != nil {
		return nil, err
	}

	if err := w.Add(path); err != nil {
		w.Close()
		return nil, err
	}

	// Drain errors channel to prevent loop goroutine from blocking.
	go func() {
		for range w.Errors() {
		}
	}()

	go func() {
		for event := range w.Events() {
			fn(event)
		}
	}()

	stop = func() error {
		return w.Close()
	}
	return stop, nil
}
