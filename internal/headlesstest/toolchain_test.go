package headlesstest

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func goEnv(t *testing.T) string {
	t.Helper()
	out, err := exec.Command(goTool(), append([]string{"env"}, toolchainEnv[:]...)...).Output()
	if err != nil {
		t.Fatalf("go env: %v", err)
	}
	return string(out)
}

func TestAPinnedToolchainKeepsItsCachesWhenTheHomeMoves(t *testing.T) {
	for _, key := range toolchainEnv {
		t.Setenv(key, os.Getenv(key))
	}
	want := goEnv(t)
	if err := PinToolchain(); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	if got := goEnv(t); got != want {
		t.Errorf("after the home moved the go command resolves\n%swant what it resolved when pinned\n%s", got, want)
	}
}
