package data

import (
	"reflect"
	"testing"
)

// TestSceneCatalogPages 验证场景鉴赏按 scene.dat 的分组列分页。
//
// 页签和分组一一对应；不能按缩略格数把整张表顺序切开，否则后一组的场景
// 会被挤进前一页（角色页里混入别的角色的封面）。
func TestSceneCatalogPages(t *testing.T) {
	catalog := &SceneCatalog{Scenes: []ReplayScene{
		{Group: 0, SceneId: "A"},
		{Group: 0, SceneId: "B"},
		{Group: 1, SceneId: "C"},
		{Group: 3, SceneId: "D"},
		{Group: 1, SceneId: "E"},
	}}
	pages := catalog.Pages()
	if len(pages) != 3 {
		t.Fatalf("%d pages, want 3", len(pages))
	}
	wantGroups := []int{0, 1, 3}
	wantScenes := [][]int{{0, 1}, {2, 4}, {3}}
	for index := range pages {
		if pages[index].Group != wantGroups[index] {
			t.Fatalf("page %d group = %d, want %d", index, pages[index].Group, wantGroups[index])
		}
		if !reflect.DeepEqual(pages[index].Scenes, wantScenes[index]) {
			t.Fatalf("page %d scenes = %v, want %v", index, pages[index].Scenes, wantScenes[index])
		}
	}
	// Scenes 本身不能因为分页被重排：全局存档的解锁位按这个行序记录。
	if catalog.Scenes[2].SceneId != "C" {
		t.Fatalf("Scenes order changed: %v", catalog.Scenes)
	}
}
