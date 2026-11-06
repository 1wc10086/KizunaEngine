package scriptdb

import "testing"

func TestParseCSVAndLookup(t *testing.T) {
	rows := ParseCSV([]byte("\xef\xbb\xbf\"調教進行度\",\"早紀\",\"ありさ\"\r\n\"従順度\",5,\"7\"\r\n"))
	if len(rows) != 2 || len(rows[0]) != 3 {
		t.Fatalf("rows=%d first=%d", len(rows), len(rows[0]))
	}
	db := &DB{tables: map[string]*table{}, raw: map[string][][]string{}, mapped: map[string]string{}}
	db.install("ID_BP", rows)
	if got := db.GetNamed("ID_BP", "従順度", "早紀"); got != 5 {
		t.Fatalf("GetNamed=%d want 5", got)
	}
	db.Set("ID_BP", db.RowIndex("ID_BP", "従順度"), "ありさ", 9)
	if got := db.GetNamed("#ID_BP", "従順度", "ありさ"); got != 9 {
		t.Fatalf("after set=%d want 9", got)
	}
}

func TestLoadLenientVarDB(t *testing.T) {
	files := map[string][]byte{
		"var_db": []byte(`{
			"common": { "ID_TERCOR": "var_territory_correction" },
			"private": { "ID_BP": "var_base_param" "ID_LOG": "var_log" }
		}`),
		"var_base_param": []byte("\"基本パラメータ\",\"早紀\"\n\"従順度\",0\n"),
		"var_log":        []byte("\"調教ログ\",\"早紀\"\n\"膣挿入回数\",0\n"),
	}
	db := Load(func(name string) ([]byte, bool) {
		content, ok := files[name]
		return content, ok
	})
	if db.TableCount() != 2 {
		t.Fatalf("tables=%d want 2", db.TableCount())
	}
	if got := db.GetNamed("ID_LOG", "膣挿入回数", "早紀"); got != 0 {
		t.Fatalf("log=%d", got)
	}
	if got := db.Add("ID_LOG", db.RowIndex("ID_LOG", "膣挿入回数"), "早紀", 3, 0, 99); got != 3 {
		t.Fatalf("add=%d", got)
	}
}
