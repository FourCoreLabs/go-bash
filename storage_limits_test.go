package gobash_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	gobash "github.com/mark3labs/go-bash"
	"github.com/mark3labs/go-bash/fs"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func TestMemFSByteBudget(t *testing.T) {
	m := memfs.New()
	if err := m.SetMaxBytes(4); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteFile("/a", []byte("1234"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := m.AppendFile("/a", []byte("5"), 0644); err == nil {
		t.Fatal("append exceeded budget")
	}
	if got, _ := m.ReadFile("/a"); string(got) != "1234" {
		t.Fatalf("failed append changed data: %q", got)
	}
	if err := m.Remove("/a"); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteFile("/b", []byte("1234"), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestNewFilesystemByteBudget(t *testing.T) {
	limit := 0
	b, err := gobash.New(gobash.BashOptions{ExecutionLimits: &gobash.ExecutionLimits{MaxFileSystemBytes: &limit}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = b.Exec(context.Background(), "echo abc > /x", gobash.ExecOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// echo ignores the redirection write error and may report success. Verify
	// persisted bytes rather than relying on its exit status.
	if data, err := b.FS().ReadFile("/x"); err == nil && len(data) != 0 {
		t.Fatalf("over-budget data persisted: %q", data)
	}
	_, err = b.Exec(context.Background(), "printf '12345' > /y", gobash.ExecOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if data, err := b.FS().ReadFile("/y"); err == nil && len(data) != 0 {
		t.Fatalf("over-budget data persisted: %q", data)
	}
}

func TestBudgetZeroAndCustomFS(t *testing.T) {
	zero := 0
	if _, err := gobash.New(gobash.BashOptions{ExecutionLimits: &gobash.ExecutionLimits{MaxFileSystemBytes: &zero}, Files: map[string]fs.FileInit{"/x": {Content: []byte("x")}}}); err == nil {
		t.Fatal("seed exceeded zero budget")
	}
	if _, err := gobash.New(gobash.BashOptions{FS: memfs.New(), ExecutionLimits: &gobash.ExecutionLimits{MaxFileSystemBytes: &zero}}); err != nil {
		t.Fatal(err)
	}
}

func TestMemFSBudgetHardlinksRenameAndLazy(t *testing.T) {
	m := memfs.New()
	if err := m.SetMaxBytes(5); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteFile("/a", []byte("1234"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := m.Link("/a", "/hard"); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove("/a"); err != nil {
		t.Fatal(err)
	}
	if err := m.AppendFile("/hard", []byte("5"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := m.Rename("/hard", "/renamed"); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteFile("/over", []byte("x"), 0644); err == nil {
		t.Fatal("hardlink bytes not charged after rename")
	}
	lazy := memfs.New()
	if err := lazy.Seed(map[string]fs.FileInit{"/lazy": {Lazy: func(context.Context) ([]byte, error) { return []byte("1234"), nil }}}); err != nil {
		t.Fatal(err)
	}
	if err := lazy.SetMaxBytes(3); err != nil {
		t.Fatal(err)
	}
	if _, err := lazy.ReadFile("/lazy"); err == nil {
		t.Fatal("lazy data exceeded budget")
	}
}

func TestMemFSBudgetConcurrentWriters(t *testing.T) {
	m := memfs.New()
	if err := m.SetMaxBytes(1); err != nil {
		t.Fatal(err)
	}
	const count = 12
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := range count {
		wg.Add(1)
		go func(i int) { defer wg.Done(); errs <- m.WriteFile(fmt.Sprintf("/%d", i), []byte("x"), 0644) }(i)
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("concurrent writes deadlocked")
	}
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("successful writes = %d, want 1", success)
	}
}

func TestMemFSBudgetWriteRemoveDoesNotDeadlock(t *testing.T) {
	m := memfs.New()
	if err := m.WriteFile("/x", []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	h, err := m.OpenFile("/x", 2, 0644)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 100 {
			_, _ = h.Write([]byte("y"))
			_ = m.Remove("/x")
			_ = m.WriteFile("/x", []byte("x"), 0644)
		}
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("write/remove lock inversion deadlocked")
	}
}
