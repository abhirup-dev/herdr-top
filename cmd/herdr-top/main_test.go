package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestHelpAndVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := run([]string{"--help"}, &out, &errOut); err != nil || !strings.Contains(errOut.String(), "Usage: herdr-top") {
		t.Fatalf("help err=%v output=%q", err, errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if err := run([]string{"--version"}, &out, &errOut); err != nil || strings.TrimSpace(out.String()) != "dev" {
		t.Fatalf("version err=%v output=%q", err, out.String())
	}
}

func TestRejectsInvalidOptionsBeforeCollection(t *testing.T) {
	for _, args := range [][]string{{"--json", "--list"}, {"--sort", "bogus"}, {"--sample", "0"}} {
		var out, stderr bytes.Buffer
		if err := run(args, &out, &stderr); err == nil {
			t.Fatalf("args %v succeeded", args)
		}
	}
}
