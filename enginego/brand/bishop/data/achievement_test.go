package data

import "testing"

func TestParseAchievementTable(t *testing.T) {
	table := ParseAchievementTable([]byte(`{
		"ID00": { "secret": FALSE, "name": "序曲", "notes": "オープニングを見た。" },
		"ID03": { "secret": TRUE, "name":"親愛", "notes": "エンディング。" },
	}`))
	if table.Len() != 2 {
		t.Fatalf("len=%d", table.Len())
	}
	entry, ok := table.Get("ID00")
	if !ok || entry.Name != "序曲" || entry.Secret {
		t.Fatalf("ID00=%+v", entry)
	}
	secret, _ := table.Get("ID03")
	if !secret.Secret || secret.Notes != "エンディング。" {
		t.Fatalf("ID03=%+v", secret)
	}
}
