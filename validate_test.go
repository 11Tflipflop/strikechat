package main

import (
	"strings"
	"testing"
)

func TestCleanName(t *testing.T) {
	if cleanName("   ") != "anon" {
		t.Fatal("empty name should become anon")
	}
	if got := cleanName(strings.Repeat("x", 50)); len([]rune(got)) != maxName {
		t.Fatalf("name not cut to %d: %q", maxName, got)
	}
	if got := cleanName("bob\u202e\x00"); got != "bob" {
		t.Fatalf("control/bidi chars not removed: %q", got)
	}
}

func TestCleanText(t *testing.T) {
	if _, ok := cleanText("   \n "); ok {
		t.Fatal("blank text should be rejected")
	}
	if _, ok := cleanText(strings.Repeat("a", maxText+1)); ok {
		t.Fatal("too long text should be rejected")
	}
	got, ok := cleanText("hi\nthere\x07")
	if !ok || got != "hi\nthere" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
}
