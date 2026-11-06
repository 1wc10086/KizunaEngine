package providers

import (
	"testing"
	"time"
)

func TestDetectBranches(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"https://generativelanguage.googleapis.com/", "google"},
		{"https://GENERATIVELANGUAGE.GOOGLEAPIS.COM/v1beta", "google"},
		{"https://api.anthropic.com", "claude"},
		{"https://gateway.example.com/anthropic/v1", "claude"},
		{"https://api.openai.com/v1", "openai"},
		{"https://example.com/v1", "openai"},
		{"https://gateway.example.com/openai", "openai"},
	}
	for _, c := range cases {
		got, err := Detect(c.url, "key")
		if err != nil {
			t.Fatalf("Detect(%q) error: %v", c.url, err)
		}
		if got != c.want {
			t.Errorf("Detect(%q) = %q, want %q", c.url, got, c.want)
		}
	}
	if _, err := Detect("", "key"); err == nil {
		t.Error("Detect(\"\") expected error, got nil")
	}
	if _, err := Detect("   ", "key"); err == nil {
		t.Error("Detect(blank) expected error, got nil")
	}
}

func TestNewKinds(t *testing.T) {
	for _, kind := range []string{"openai", "google", "claude"} {
		p, err := New(Spec{Kind: kind})
		if err != nil {
			t.Errorf("New(%q) error: %v", kind, err)
			continue
		}
		if p == nil {
			t.Errorf("New(%q) returned nil provider", kind)
		}
	}
	if _, err := New(Spec{Kind: "bogus"}); err == nil {
		t.Error("New(bogus) expected error, got nil")
	}
	if _, err := New(Spec{}); err == nil {
		t.Error("New(empty) expected error, got nil")
	}
}

func TestClientTimeoutDefault(t *testing.T) {
	p, err := New(Spec{Kind: "openai"})
	if err != nil {
		t.Fatal(err)
	}
	if got := p.(*openaiProvider).client.Timeout; got != 120*time.Second {
		t.Errorf("default timeout = %v, want 120s", got)
	}
	p, err = New(Spec{Kind: "openai", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if got := p.(*openaiProvider).client.Timeout; got != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", got)
	}
}
