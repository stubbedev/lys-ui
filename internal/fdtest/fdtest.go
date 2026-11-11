// Package fdtest reads what a process's file descriptors refer to, for
// tests that hold a component to closing every descriptor it is
// handed.
package fdtest

import (
	"errors"
	"io/fs"
	"os"
	"regexp"
	"slices"
	"strconv"
	"testing"
)

var numberedInode = regexp.MustCompile(`^(socket|pipe):\[\d+\]$`)

// Held is what process pid's open file descriptors refer to, in order:
// the path each /proc/<pid>/fd entry links to, a socket and a pipe
// without their inode numbers, so that two readings compare by what is
// open rather than by which descriptor numbers hold it.
func Held(t testing.TB, pid int) []string {
	t.Helper()
	dir := "/proc/" + strconv.Itoa(pid) + "/fd"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	held := make([]string, 0, len(entries))
	for _, entry := range entries {
		target, err := os.Readlink(dir + "/" + entry.Name())
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, numberedInode.ReplaceAllString(target, "$1"))
	}
	slices.Sort(held)
	return held
}

// Own is Held for the test's own process.
func Own(t testing.TB) []string {
	t.Helper()
	return Held(t, os.Getpid())
}

// Beyond lists what held has more of than baseline, both in order.
func Beyond(held, baseline []string) []string {
	var extra []string
	for _, target := range held {
		for len(baseline) > 0 && baseline[0] < target {
			baseline = baseline[1:]
		}
		if len(baseline) > 0 && baseline[0] == target {
			baseline = baseline[1:]
			continue
		}
		extra = append(extra, target)
	}
	return extra
}

// RequireNoneBeyond fails the test when its own process holds anything
// beyond baseline, naming what it holds and after what.
func RequireNoneBeyond(t testing.TB, baseline []string, after string) {
	t.Helper()
	if extra := Beyond(Own(t), baseline); len(extra) != 0 {
		t.Errorf("after %s the process holds %q beyond what it did", after, extra)
	}
}
