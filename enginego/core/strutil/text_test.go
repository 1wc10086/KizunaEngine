package strutil

import (
	"reflect"
	"testing"
)

func TestNormalizeEOL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"plain text", "plain text"},
		{"a\nb", "a\nb"},
		{"a\r\nb", "a\nb"},
		{"a\rb", "a\nb"},
		{"a\r\nb\rc\nd", "a\nb\nc\nd"},
		{"\r\n\r\n", "\n\n"},
		{"line1\r\nline2\rline3\nline4", "line1\nline2\nline3\nline4"},
	}
	for _, c := range cases {
		if got := NormalizeEOL(c.in); got != c.want {
			t.Fatalf("NormalizeEOL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTolerantAtoi(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"", 0, false},
		{"   ", 0, false},
		{"\t\n\r ", 0, false},
		{"42", 42, true},
		{"  42", 42, true},
		{"\t\n 42 ", 42, true},
		{"+42", 42, true},
		{"-42", -42, true},
		{"+ 42", 0, false},
		{"- 42", 0, false},
		{"42abc", 42, true},
		{"42.9", 42, true},
		{"-42 rest of line", -42, true},
		{"abc", 0, false},
		{"+", 0, false},
		{"-", 0, false},
		{"--7", 0, false},
		{"++7", 0, false},
		{"0", 0, true},
		{"-0", 0, true},
		{"00042", 42, true},
		{".5", 0, false},
		{"\u00a05", 5, true},
		{"9999999999", 9999999999, true},
	}
	for _, c := range cases {
		got, ok := TolerantAtoi(c.in)
		if got != c.want || ok != c.ok {
			t.Fatalf("TolerantAtoi(%q) = (%d, %v), want (%d, %v)", c.in, got, ok, c.want, c.ok)
		}
	}

	if _, ok := TolerantAtoi("1234567890123456789012345678901234567890"); !ok {
		t.Fatalf("TolerantAtoi accepted no digits from an overflowing digit run")
	}
}

func TestSplitTrim(t *testing.T) {
	cases := []struct {
		in   string
		sep  string
		want []string
	}{
		{" a , b ,c ", ",", []string{"a", "b", "c"}},
		{"a,,b", ",", []string{"a", "", "b"}},
		{"", ",", []string{""}},
		{"a|b", "|", []string{"a", "b"}},
		{" a ; b ", ";", []string{"a", "b"}},
	}
	for _, c := range cases {
		if got := SplitTrim(c.in, c.sep); !reflect.DeepEqual(got, c.want) {
			t.Fatalf("SplitTrim(%q, %q) = %#v, want %#v", c.in, c.sep, got, c.want)
		}
	}

	nonEmptyCases := []struct {
		in   string
		sep  string
		want []string
	}{
		{" a , b ,c ", ",", []string{"a", "b", "c"}},
		{"a,,b", ",", []string{"a", "b"}},
		{"a, ,b,", ",", []string{"a", "b"}},
		{"", ",", []string{}},
		{"   ", ",", []string{}},
	}
	for _, c := range nonEmptyCases {
		if got := SplitTrimNonEmpty(c.in, c.sep); !reflect.DeepEqual(got, c.want) {
			t.Fatalf("SplitTrimNonEmpty(%q, %q) = %#v, want %#v", c.in, c.sep, got, c.want)
		}
	}
}

func TestTrimQuotes(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`abc`, `abc`},
		{`"abc"`, `abc`},
		{`'abc'`, `abc`},
		{`"abc'`, `"abc'`},
		{`'abc"`, `'abc"`},
		{`"`, `"`},
		{`""`, ``},
		{`''`, ``},
		{`@abc`, `abc`},
		{`@`, ``},
		{`@"abc"`, `abc`},
		{`"@abc"`, `abc`},
		{`@@abc`, `@abc`},
		{`@'abc'`, `abc`},
	}
	for _, c := range cases {
		if got := TrimQuotes(c.in); got != c.want {
			t.Fatalf("TrimQuotes(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFoldHelpers(t *testing.T) {
	if !HasPrefixFold("Scene.PFT", "scene") {
		t.Fatalf("HasPrefixFold did not match ignoring case")
	}
	if !HasPrefixFold("Scene.PFT", "Scene") {
		t.Fatalf("HasPrefixFold did not match equal case")
	}
	if HasPrefixFold("sc", "scene") {
		t.Fatalf("HasPrefixFold matched a longer prefix")
	}
	if !HasPrefixFold("Scene.PFT", "") {
		t.Fatalf("HasPrefixFold did not match an empty prefix")
	}

	if !HasSuffixFold("Image.PNG", ".png") {
		t.Fatalf("HasSuffixFold did not match ignoring case")
	}
	if HasSuffixFold("Image.PNG", ".pngx") {
		t.Fatalf("HasSuffixFold matched a longer suffix")
	}

	if !HasAnySuffixFold("Image.PNG", []string{".bmp", ".png"}) {
		t.Fatalf("HasAnySuffixFold did not find a matching suffix")
	}
	if HasAnySuffixFold("Image.PNG", []string{".bmp", ".jpg"}) {
		t.Fatalf("HasAnySuffixFold matched an unrelated suffix")
	}
	if HasAnySuffixFold("Image.PNG", nil) {
		t.Fatalf("HasAnySuffixFold matched an empty suffix list")
	}
}
