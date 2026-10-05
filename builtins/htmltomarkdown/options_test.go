package htmltomarkdown_test

import (
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func TestCustomizationFlags(t *testing.T) {
	tests := []struct {
		name, input string
		args        []string
		want        string
	}{
		{"bullet short", "<ul><li>Item</li></ul>", []string{"-b", "+"}, "+ Item"},
		{"bullet long", "<ul><li>Item</li></ul>", []string{"--bullet", "*"}, "* Item"},
		{"bullet equals", "<ul><li>Item</li></ul>", []string{"--bullet=+"}, "+ Item"},
		{"bullet default", "<ul><li>Item</li></ul>", nil, "- Item"},
		{"code short", "<pre><code>code</code></pre>", []string{"-c", "~~~"}, "~~~\ncode\n~~~"},
		{"code long", "<pre><code>code</code></pre>", []string{"--code", "~~~"}, "~~~\ncode\n~~~"},
		{"code equals", "<pre><code>code</code></pre>", []string{"--code=~~~"}, "~~~\ncode\n~~~"},
		{"code default", "<pre><code>code</code></pre>", nil, "```\ncode\n```"},
		{"hr short", "<hr>", []string{"-r", "***"}, "***"},
		{"hr long", "<hr>", []string{"--hr", "___"}, "___"},
		{"hr equals", "<hr>", []string{"--hr=***"}, "***"},
		{"hr default", "<hr>", nil, "---"},
		{"hr arbitrary", "<hr>", []string{"--hr=divider"}, "divider"},
		{"hr empty", "<hr>", []string{"--hr="}, ""},
		{"setext", "<h1>Title</h1><h2>Other</h2><h3>Small</h3>", []string{"--heading-style=setext"}, "Title\n=====\n\nOther\n-----\n\n### Small"},
		{"atx", "<h1>Title</h1>", []string{"--heading-style=atx"}, "# Title"},
		{"invalid style ignored", "<h1>Title</h1>", []string{"--heading-style=unknown"}, "# Title"},
		{"last valid style retained", "<h1>Title</h1>", []string{"--heading-style=setext", "--heading-style=unknown"}, "Title\n====="},
		{"last marker wins", "<ul><li>Item</li></ul>", []string{"-b", "*", "--bullet=+"}, "+ Item"},
		{"missing bullet defaults", "<ul><li>Item</li></ul>", []string{"-b"}, "- Item"},
		{"missing code defaults", "<pre><code>code</code></pre>", []string{"--code"}, "```\ncode\n```"},
		{"missing hr defaults", "<hr>", []string{"-r"}, "---"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, errOut, code := runCmd(t, &command.Context{Stdin: strings.NewReader(tt.input)}, tt.args...)
			if out != tt.want || errOut != "" || code != 0 {
				t.Fatalf("got (%q, %q, %d), want (%q, empty, 0)", out, errOut, code, tt.want)
			}
		})
	}
}

func TestInvalidCustomization(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want string
	}{
		{[]string{"--bullet=x"}, "invalid bullet marker"},
		{[]string{"--bullet="}, "invalid bullet marker"},
		{[]string{"-b", "--code=~~~"}, "invalid bullet marker"},
		{[]string{"--code=invalid"}, "invalid code fence"},
		{[]string{"--code="}, "invalid code fence"},
	} {
		out, errOut, code := runCmd(t, &command.Context{Stdin: strings.NewReader("<p>text</p>")}, tt.args...)
		if out != "" || errOut != "html-to-markdown: "+tt.want+"\n" || code != 1 {
			t.Errorf("args=%q got (%q, %q, %d)", tt.args, out, errOut, code)
		}
		out, errOut, code = runCmd(t, &command.Context{Stdin: strings.NewReader(" \n")}, tt.args...)
		if out != "" || errOut != "" || code != 0 {
			t.Errorf("empty input args=%q got (%q, %q, %d)", tt.args, out, errOut, code)
		}
	}
	// Upstream supports only the equals form for heading-style.
	_, _, code := runCmd(t, nil, "--heading-style", "setext")
	if code != 2 {
		t.Errorf("code=%d want 2", code)
	}
}

func TestCustomizationFileAndHelp(t *testing.T) {
	fsx := memfs.New()
	if err := fsx.WriteFile("/page.html", []byte("<h1>Title</h1><hr>"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := runCmd(t, &command.Context{FS: fsx, Cwd: "/"}, "page.html", "--heading-style=setext", "--hr=***")
	if out != "Title\n=====\n\n***" || errOut != "" || code != 0 {
		t.Fatalf("got (%q, %q, %d)", out, errOut, code)
	}
	out, errOut, code = runCmd(t, nil, "--bogus", "--help")
	if code != 0 || errOut != "" {
		t.Fatalf("help: (%q, %q, %d)", out, errOut, code)
	}
	for _, flag := range []string{"--bullet", "--code", "--hr", "--heading-style"} {
		if !strings.Contains(out, flag) {
			t.Errorf("help missing %s", flag)
		}
	}
}
