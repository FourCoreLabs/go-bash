package interp

import (
	"reflect"
	"unsafe"

	mvinterp "mvdan.cc/sh/v3/interp"
)

// activeRunner returns the *interp.Runner that is currently executing,
// as carried by the HandlerContext.
//
// Why this exists: `cd` has to move the interpreter's Dir, and the
// interpreter that must move is whichever one is running right now —
// a subshell has its own Runner with its own Dir, and mutating the
// root Runner instead would both fail to take effect inside the
// subshell and leak the change out of it. mvdan/sh keeps the active
// Runner on HandlerContext (it powers the exported
// HandlerContext.Builtin) but does not export the field.
//
// This is deliberately the only place in gobash that reaches for
// unsafe. It is a read of one pointer field on a struct we already
// hold, it never escapes this package, and it fails closed: if the
// field is missing or changes type in a future mvdan/sh, the lookup
// returns nil and `cd` reports an error rather than silently writing
// to the wrong Runner.
//
// The clean long-term fix is upstream: mvdan/sh's os_unix.go already
// carries `TODO(v4): "access" may need to become part of a handler,
// like "open" or "stat"`. Once access() is handler-routed, mvdan's own
// cd works against the VFS and this file can be deleted.
func activeRunner(hc mvinterp.HandlerContext) *mvinterp.Runner {
	v := reflect.ValueOf(hc)
	if v.Kind() != reflect.Struct {
		return nil
	}
	f := v.FieldByName("runner")
	if !f.IsValid() || f.Kind() != reflect.Pointer {
		return nil
	}
	if f.Type() != reflect.TypeOf((*mvinterp.Runner)(nil)) {
		return nil
	}
	// The field is addressable only through an addressable copy.
	cp := reflect.New(v.Type()).Elem()
	cp.Set(v)
	fc := cp.FieldByName("runner")
	ptr := unsafe.Pointer(fc.UnsafeAddr())
	return *(**mvinterp.Runner)(ptr)
}
