package main

import (
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

// TestExtraInfoRoundTrip guards the marshalling used by the rs-info channel:
// the victim marshals ExtraInfo, the attacker unmarshals it. If the struct or
// its wire encoding ever drifts, the "New connection from ..." line breaks.
func TestExtraInfoRoundTrip(t *testing.T) {
	in := ExtraInfo{
		CurrentUser:      "root",
		Hostname:         "victim-01",
		ListeningAddress: "127.0.0.1:8888",
	}

	var out ExtraInfo
	if err := gossh.Unmarshal(gossh.Marshal(&in), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out != in {
		t.Fatalf("round trip mismatch:\n got  %+v\n want %+v", out, in)
	}
}

// TestCreatePublicKeyHandlerNilWhenEmpty documents that an empty authorized key
// disables public-key auth (leaving password auth as the only method).
func TestCreatePublicKeyHandlerNilWhenEmpty(t *testing.T) {
	if createPublicKeyHandler("") != nil {
		t.Fatal("expected nil handler for empty authorized key")
	}
	if createPublicKeyHandler("ssh-ed25519 AAAA... test") == nil {
		t.Fatal("expected non-nil handler for a configured authorized key")
	}
}
