package session

import "testing"

func TestHostRequestURL(t *testing.T) {
	s := New()
	s.setHostRequestURL("  http://example.com/enq  ")
	if s.hostRequest != "url:http://example.com/enq" {
		t.Fatalf("hostRequest = %q", s.hostRequest)
	}
	if s.hostRequestSerial != 1 {
		t.Fatalf("serial = %d, want 1", s.hostRequestSerial)
	}
	s.setHostRequestURL("   ")
	if s.hostRequestSerial != 1 {
		t.Fatal("empty URL must not bump the serial")
	}
}
