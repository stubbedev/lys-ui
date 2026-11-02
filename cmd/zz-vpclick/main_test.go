package main

import (
	"strings"
	"testing"
)

func TestParseRejectsBadCommandLines(t *testing.T) {
	for _, args := range []string{
		"", "1", "-1 2", "1 2147483648", "1 x",
		"1 2 key", "1 2 key -5", "1 2 key 65536", "1 2 combo", "1 2 combo -5",
		"1 2 scroll", "1 2 scroll down", "1 2 type", "1 2 click 3", "1 2 bogus",
	} {
		if act, err := parse(strings.Fields(args)); err == nil || act != nil {
			t.Errorf("parse(%q) accepted a bad command line", args)
		}
	}
}

func TestParseAcceptsEveryAction(t *testing.T) {
	for _, args := range []string{
		"1 2", "1 2 click", "1 2 dclick", "1 2 scroll -1.5", "1 2 key 28", "1 2 combo 28", "1 2 type hi",
	} {
		if act, err := parse(strings.Fields(args)); err != nil || act == nil {
			t.Errorf("parse(%q) = %v", args, err)
		}
	}
}
