package file

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcherCreateEvent(t *testing.T) {
	dir := t.TempDir()

	w, err := NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}
	defer w.Close()

	if err := w.Add(dir); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	done := make(chan Event, 1)
	go func() {
		select {
		case e := <-w.Events():
			done <- e
		case <-time.After(3 * time.Second):
		}
	}()

	// Give watcher time to start
	time.Sleep(100 * time.Millisecond)
	os.WriteFile(filepath.Join(dir, "new.txt"), []byte("x"), 0644)

	e := <-done
	if e.Path == "" {
		t.Fatal("event path is empty")
	}
	if e.Op&Create == 0 && e.Op&Write == 0 {
		t.Fatalf("event op = %s, want create or write", e.Op)
	}
}

func TestWatcherFilter(t *testing.T) {
	dir := t.TempDir()

	w, err := NewWatcher(WithFilter(Remove))
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}
	defer w.Close()

	w.Add(dir)

	// Create a file - should be filtered out
	time.Sleep(100 * time.Millisecond)
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("x"), 0644)

	// Check no event comes through for create
	select {
	case e := <-w.Events():
		t.Fatalf("unexpected event: %s %s", e.Op, e.Path)
	case <-time.After(500 * time.Millisecond):
		// expected - create events are filtered
	}
}

func TestWatcherDebounce(t *testing.T) {
	dir := t.TempDir()

	w, err := NewWatcher(WithDebounce(200 * time.Millisecond))
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}
	defer w.Close()

	w.Add(dir)
	time.Sleep(100 * time.Millisecond)

	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("v1"), 0644)
	time.Sleep(50 * time.Millisecond)
	os.WriteFile(path, []byte("v2"), 0644)
	time.Sleep(50 * time.Millisecond)
	os.WriteFile(path, []byte("v3"), 0644)

	// Should get debounced events (create + one debounced write)
	var events []Event
	timeout := time.After(1 * time.Second)
	for {
		select {
		case e := <-w.Events():
			events = append(events, e)
		case <-timeout:
			goto done
		}
	}
done:
	if len(events) == 0 {
		t.Fatal("no events received")
	}
}

func TestWatcherClose(t *testing.T) {
	dir := t.TempDir()

	w, err := NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}
	w.Add(dir)

	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Double close should be fine
	if err := w.Close(); err != nil {
		t.Fatalf("Close() second call error = %v", err)
	}
}

func TestWatcherAddRemoveAfterClose(t *testing.T) {
	dir := t.TempDir()

	w, err := NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}
	if err := w.Add(dir); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := w.Add(dir); err == nil {
		t.Fatal("Add() after Close error = nil, want error")
	}
	if err := w.Remove(dir); err == nil {
		t.Fatal("Remove() after Close error = nil, want error")
	}
}

func TestWatch(t *testing.T) {
	dir := t.TempDir()

	received := make(chan Event, 1)
	stop, err := Watch(dir, func(e Event) {
		received <- e
	})
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	defer stop()

	time.Sleep(100 * time.Millisecond)
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("x"), 0644)

	select {
	case e := <-received:
		if e.Path == "" {
			t.Fatal("event path is empty")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestOpString(t *testing.T) {
	tests := []struct {
		op   Op
		want string
	}{
		{Create, "create"},
		{Write, "write"},
		{Remove, "remove"},
		{Rename, "rename"},
		{Chmod, "chmod"},
	}
	for _, tt := range tests {
		if got := tt.op.String(); got != tt.want {
			t.Fatalf("Op.String() = %q, want %q", got, tt.want)
		}
	}
}
