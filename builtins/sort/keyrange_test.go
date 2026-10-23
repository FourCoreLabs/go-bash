package sort_test

import (
	"strings"
	"testing"
)

// A key range whose end is before its start names no field. It must sort on
// the empty key like GNU sort, never panic: the panic is raised in the
// interpreter's own pipeline goroutine, where no caller can recover it. The
// start field has to exist for the slice to go backwards, so every line here
// has at least as many fields as the range's start.
func TestKeyRangeEndingBeforeItStarts(t *testing.T) {
	for _, tt := range []struct {
		args []string
		in   string
	}{
		{[]string{"-k3,1"}, "b x 2\na y 1\n"},
		{[]string{"-k7,2"}, "b 1 2 3 4 5 6\na 1 2 3 4 5 6\n"},
		{[]string{"-t,", "-k3,1"}, "b,x,2\na,y,1\n"},
	} {
		out, err, code := run(t, tt.in, tt.args...)
		if code != 0 || err != "" {
			t.Errorf("args=%v code=%d err=%q", tt.args, code, err)
		}
		for _, line := range strings.Split(strings.TrimSpace(tt.in), "\n") {
			if !strings.Contains(out, line) {
				t.Errorf("args=%v lost line %q: %q", tt.args, line, out)
			}
		}
	}
}
