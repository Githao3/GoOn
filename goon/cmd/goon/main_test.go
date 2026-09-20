package main

import "testing"

func TestDispatch_Unknown(t *testing.T) {
	_, err := dispatch([]string{"bogus"})
	if err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
}

func TestDispatch_NoArgs(t *testing.T) {
	_, err := dispatch(nil)
	if err == nil {
		t.Fatal("expected usage error when no subcommand given")
	}
}

func TestDispatch_Version(t *testing.T) {
	out, err := dispatch([]string{"version"})
	if err != nil || out != version {
		t.Fatalf("got out=%q err=%v", out, err)
	}
}
