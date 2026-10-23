package fs

import (
	"io"
	iofs "io/fs"
	"os"
	"path"
	"time"
)

// NullPath is the canonical absolute path of the null device.
const NullPath = "/dev/null"

// WithNullDevice wraps inner so that NullPath behaves like a character
// device instead of an ordinary file: reads return EOF immediately and
// writes are discarded.
//
// Without this, `cmd >/dev/null` creates a regular file in the VFS that
// silently accumulates everything written to it, and a later
// `cat /dev/null` hands that content back — which is exactly the
// opposite of what every script using /dev/null expects. The wrapper
// sits at the FileSystem layer rather than in the interpreter's open
// handler so that built-ins reading and writing through Context.FS see
// the same behavior as shell redirections.
func WithNullDevice(inner FileSystem) FileSystem {
	if inner == nil {
		return nil
	}
	return nullDeviceFS{FileSystem: inner}
}

// nullDeviceFS embeds FileSystem so every method it does not override
// forwards to the wrapped implementation unchanged.
type nullDeviceFS struct{ FileSystem }

func isNullPath(name string) bool { return path.Clean(name) == NullPath }

func (f nullDeviceFS) Open(name string) (iofs.File, error) {
	if isNullPath(name) {
		return nullFile{}, nil
	}
	return f.FileSystem.Open(name)
}

func (f nullDeviceFS) OpenFile(name string, flag int, perm os.FileMode) (File, error) {
	if isNullPath(name) {
		return nullFile{}, nil
	}
	return f.FileSystem.OpenFile(name, flag, perm)
}

func (f nullDeviceFS) Create(name string) (File, error) {
	if isNullPath(name) {
		return nullFile{}, nil
	}
	return f.FileSystem.Create(name)
}

func (f nullDeviceFS) ReadFile(name string) ([]byte, error) {
	if isNullPath(name) {
		return nil, nil
	}
	return f.FileSystem.ReadFile(name)
}

func (f nullDeviceFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	if isNullPath(name) {
		return nil
	}
	return f.FileSystem.WriteFile(name, data, perm)
}

func (f nullDeviceFS) AppendFile(name string, data []byte, perm os.FileMode) error {
	if isNullPath(name) {
		return nil
	}
	return f.FileSystem.AppendFile(name, data, perm)
}

func (f nullDeviceFS) Stat(name string) (os.FileInfo, error) {
	if isNullPath(name) {
		return nullInfo{}, nil
	}
	return f.FileSystem.Stat(name)
}

func (f nullDeviceFS) Lstat(name string) (os.FileInfo, error) {
	if isNullPath(name) {
		return nullInfo{}, nil
	}
	return f.FileSystem.Lstat(name)
}

func (f nullDeviceFS) Remove(name string) error {
	if isNullPath(name) {
		return nil
	}
	return f.FileSystem.Remove(name)
}

// nullFile is an open handle on the null device.
type nullFile struct{}

func (nullFile) Read([]byte) (int, error)       { return 0, io.EOF }
func (nullFile) Write(p []byte) (int, error)    { return len(p), nil }
func (nullFile) Seek(int64, int) (int64, error) { return 0, nil }
func (nullFile) Truncate(int64) error           { return nil }
func (nullFile) Close() error                   { return nil }
func (nullFile) Stat() (iofs.FileInfo, error)   { return nullInfo{}, nil }

// nullInfo reports /dev/null as a zero-length character device, which
// is what `stat` and the `[ -c ]` / `[ -e ]` tests expect to see.
type nullInfo struct{}

func (nullInfo) Name() string { return "null" }
func (nullInfo) Size() int64  { return 0 }
func (nullInfo) Mode() os.FileMode {
	return os.ModeDevice | os.ModeCharDevice | 0o666
}
func (nullInfo) ModTime() time.Time { return time.Time{} }
func (nullInfo) IsDir() bool        { return false }
func (nullInfo) Sys() any           { return nil }

// UnwrapNullDevice returns the FileSystem that WithNullDevice wrapped,
// or fs unchanged when it is not a wrapper. Accessors that hand the
// filesystem back to the host (Bash.FS) use this so callers see the
// object they supplied rather than the internal decorator.
func UnwrapNullDevice(fs FileSystem) FileSystem {
	if w, ok := fs.(nullDeviceFS); ok {
		return w.FileSystem
	}
	return fs
}
