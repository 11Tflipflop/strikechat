package main

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxName = 20
	maxText = 500
)

// removes control characters (except newline) and the tricks that flip text direction
func sanitize(s string, keepNewline bool) string {
	var b strings.Builder
	for _, r := range s {
		if r == '\n' && keepNewline {
			b.WriteRune(r)
			continue
		}
		if unicode.IsControl(r) {
			continue
		}
		if (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func cleanName(s string) string {
	s = strings.TrimSpace(sanitize(s, false))
	r := []rune(s)
	if len(r) > maxName {
		r = r[:maxName]
	}
	s = strings.TrimSpace(string(r))
	if s == "" {
		return "anon"
	}
	return s
}

// returns the cleaned text and false if it can't be used
func cleanText(s string) (string, bool) {
	s = strings.TrimSpace(sanitize(s, true))
	if s == "" || utf8.RuneCountInString(s) > maxText {
		return "", false
	}
	return s, true
}
