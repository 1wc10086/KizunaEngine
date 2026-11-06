package strutil

import (
	"reflect"
	"testing"
)

func TestHasAnyExtFold(t *testing.T) {
	cases := []struct {
		name string
		exts []string
		want bool
	}{
		{"a.txt", []string{".txt"}, true},
		{"a.txt", []string{".png", ".txt"}, true},
		{"a.txt", []string{".png"}, false},
		{"A.TXT", []string{".txt"}, true},
		{"a.Txt", []string{".tXT"}, true},
		{"A.TXT", []string{".PNG", ".TXT"}, true},
		{"README", []string{".txt"}, false},
		{".hidden", []string{".hidden"}, true},
		{".hidden", []string{".txt"}, false},
		{".gitignore", []string{".GITIGNORE"}, true},
		{"dir/a.txt", []string{".txt"}, true},
		{"dir.d/a", []string{".d"}, false},
		{"dir/a", []string{".txt"}, false},
		{"a.", []string{"."}, true},
		{"a.", []string{""}, false},
		{"", []string{".txt"}, false},
		{"a.txt", nil, false},
		{"a.txt", []string{}, false},
	}
	for _, c := range cases {
		if got := HasAnyExtFold(c.name, c.exts); got != c.want {
			t.Fatalf("HasAnyExtFold(%q, %#v) = %v, want %v", c.name, c.exts, got, c.want)
		}
	}
}

func TestSplitTopLevel(t *testing.T) {
	cases := []struct {
		text string
		sep  rune
		want []string
	}{
		{"a=b=c", '=', []string{"a", "b", "c"}},
		{"a,b,c", ',', []string{"a", "b", "c"}},
		{"a;b;c", ';', []string{"a", "b", "c"}},
		{"f(a,b),c", ',', []string{"f(a,b)", "c"}},
		{"[a,(b,c)]", ',', []string{"[a,(b,c)]"}},
		{"{a,b}", ',', []string{"{a,b}"}},
		{"（a,b）,c", ',', []string{"（a,b）", "c"}},
		{"［a,b］", ',', []string{"［a,b］"}},
		{`"a,b",c`, ',', []string{`"a,b"`, "c"}},
		{`＂a,b＂,c`, ',', []string{`＂a,b＂`, "c"}},
		{`"(",x`, ',', []string{`"("`, "x"}},
		{"a=(1,2)", '=', []string{"a", "(1,2)"}},
		{")a,b", ',', []string{")a", "b"}},
		{"(a,b", ',', []string{"(a,b"}},
		{"a,b,", ',', []string{"a", "b", ""}},
		{",a", ',', []string{"", "a"}},
		{"abc", ',', []string{"abc"}},
		{"", ',', []string{""}},
	}
	for _, c := range cases {
		if got := SplitTopLevel(c.text, c.sep); !reflect.DeepEqual(got, c.want) {
			t.Fatalf("SplitTopLevel(%q, %q) = %#v, want %#v", c.text, c.sep, got, c.want)
		}
	}
}
