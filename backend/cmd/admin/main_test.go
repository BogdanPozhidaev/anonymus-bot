package main

import (
	"io"
	"strings"
	"testing"
)

func TestReadBootstrapPassword(t *testing.T) {
	value, err := readPassword(strings.NewReader("fixture password\r\n"))
	if err != nil || value != "fixture password" {
		t.Fatal("valid password rejected")
	}
	for _, in := range []string{"", " \n", "secret\nextra", strings.Repeat("x", 73), "prefix\x00suffix"} {
		if _, err := readPassword(strings.NewReader(in)); err == nil {
			t.Fatal("invalid password accepted")
		}
	}
	if err := run([]string{"create-admin", "--login", "bad login"}, strings.NewReader("fixture"), io.Discard); err == nil {
		t.Fatal("invalid login accepted")
	}
}
