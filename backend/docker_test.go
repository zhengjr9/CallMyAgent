package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNetrcFromTokenUsesRepoHost(t *testing.T) {
	content, err := netrcFromToken("https://github.com/example/private.git", "ghp_test")
	if err != nil {
		t.Fatalf("netrcFromToken returned error: %v", err)
	}
	for _, want := range []string{
		"machine github.com",
		"login x-access-token",
		"password ghp_test",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected %q in netrc content:\n%s", want, content)
		}
	}
}

func TestPrepareDockerNetrcWritesPrivateFile(t *testing.T) {
	dir := t.TempDir()
	path, err := prepareDockerNetrc(&Task{
		GitRepo:  "https://gitlab.com/example/private.git",
		GitToken: "glpat-test",
	}, dir)
	if err != nil {
		t.Fatalf("prepareDockerNetrc returned error: %v", err)
	}
	if path != filepath.Join(dir, ".netrc") {
		t.Fatalf("unexpected netrc path: %s", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat netrc: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("netrc permissions = %o, want 600", got)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read netrc: %v", err)
	}
	if !strings.Contains(string(content), "machine gitlab.com") {
		t.Fatalf("expected gitlab.com host in netrc:\n%s", content)
	}
}
