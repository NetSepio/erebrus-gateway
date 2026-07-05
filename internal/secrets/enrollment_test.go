package secrets

import "testing"

func TestNewNodeRegistrationToken(t *testing.T) {
	tok, err := NewNodeRegistrationToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(tok) < len(NodeRegistrationPrefix)+16 {
		t.Fatalf("token too short: %q", tok)
	}
	if tok[:len(NodeRegistrationPrefix)] != NodeRegistrationPrefix {
		t.Fatalf("prefix = %q", tok)
	}
}

func TestNewNodeKey(t *testing.T) {
	k, err := NewNodeKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(k) < 20 {
		t.Fatalf("node key too short: %q", k)
	}
}