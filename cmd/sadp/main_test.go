package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestMainRun_HelpSuccess(t *testing.T) {
	var buf bytes.Buffer
	code := mainRun([]string{"help"}, &buf)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
}

func TestMainRun_UnknownCommandExits1(t *testing.T) {
	var buf bytes.Buffer
	code := mainRun([]string{"nonexistent-command"}, &buf)
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if !strings.Contains(buf.String(), "Error:") {
		t.Errorf("stderr = %q, want 'Error:' prefix", buf.String())
	}
}
