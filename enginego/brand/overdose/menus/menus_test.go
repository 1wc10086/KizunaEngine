package menus

import (
	"os"
	"path/filepath"
	"testing"

	"kizuna/enginego/brand/overdose/arc"
	"kizuna/enginego/brand/overdose/save"
)

// 以下测试分两档：
//   - 纯逻辑（无游戏数据也必跑）：分组、缩略图、场景/音乐构建、
//     解锁集合、布局资源名一致性；
//   - 真数据（缺席时 Skip）：从真实 system.arc/music.arc 构建目录，
//     断言条数与首条样例与提取物（tmpwork/extract/system）一致。
// 查找顺序与 arc 测试一致，保证 CI 无游戏数据时全绿。

var gameDirCandidates = []string{
	os.Getenv("GAME_DIR"),
	"/root/code/engine/游戏/YUI/優衣",
	"../../../engine/游戏/YUI/優衣",
	"./testdata",
}

func gameDir(t *testing.T) string {
	t.Helper()
	for _, c := range gameDirCandidates {
		if c == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(c, "system.arc")); err == nil {
			return c
		}
	}
	t.Skip("未找到游戏数据目录（设置 GAME_DIR）")
	return ""
}

// fakeSource 是 Source 的内存实现，供纯逻辑测试喂表。
type fakeSource struct {
	tables map[string][]arc.Record
	names  []string
}

func (f *fakeSource) Table(name string) ([]arc.Record, error) {
	recs, ok := f.tables[name]
	if !ok {
		return nil, &fakeError{msg: "no table " + name}
	}
	return recs, nil
}

func (f *fakeSource) Names(ext string) []string { return f.names }

type fakeError struct{ msg string }

func (e *fakeError) Error() string { return e.msg }

func recs(names ...string) []arc.Record {
	out := make([]arc.Record, 0, len(names))
	for _, n := range names {
		out = append(out, arc.Record{Name: n})
	}
	return out
}

func TestCgPrefix(t *testing.T) {
	cases := map[string]string{
		"cga01-01": "cga01",
		"cgxb1-01": "cgxb1",
		"cgt05-01": "cgt05",
		"cgk01-19": "cgk01",
	}
	for in, want := range cases {
		got, ok := CgPrefix(in)
		if !ok || got != want {
			t.Errorf("CgPrefix(%q) = %q,%v want %q,true", in, got, ok, want)
		}
	}
	// 非变体：_blur 后缀、背景、系统图都不算。
	for _, in := range []string{"cga02-04_blur", "bg011b", "black", "cinemascope", "", "cg", "cga01", "cga01-", "-01"} {
		if _, ok := CgPrefix(in); ok {
			t.Errorf("CgPrefix(%q) 不应命中", in)
		}
		if IsCgVariant(in) {
			t.Errorf("IsCgVariant(%q) 应为 false", in)
		}
	}
	if ThumbName("cga01-01") != "thumb_cga01-01" {
		t.Errorf("ThumbName 变体名错误")
	}
	if ThumbName("thumb_cga01-01") != "thumb_cga01-01" {
		t.Errorf("ThumbName 已带前缀应不变")
	}
	if ThumbName("") != "" {
		t.Errorf("ThumbName 空应为空")
	}
}

func TestBuildCgGroupsPreservesOrder(t *testing.T) {
	groups := BuildCgGroups(recs(
		"black", "cinemascope", "white",
		"cga01-01", "cga01-02", "cga01-03",
		"cga02-01", "cga02-02", "cga02-04_blur", // _blur 不算变体
		"bg001a", "ed001", "tak_a01",
		"cgxb1-01", "cgxb1-02",
	))
	if len(groups) != 3 {
		t.Fatalf("groups = %d, want 3: %+v", len(groups), groups)
	}
	if groups[0].Name != "cga01" || len(groups[0].Variants) != 3 {
		t.Fatalf("cga01 组错误: %+v", groups[0])
	}
	if groups[0].Variants[0] != "cga01-01" || groups[0].Variants[2] != "cga01-03" {
		t.Fatalf("cga01 变体顺序错误: %v", groups[0].Variants)
	}
	if groups[0].Thumb != "thumb_cga01-01" {
		t.Fatalf("Thumb = %q", groups[0].Thumb)
	}
	if groups[1].Name != "cga02" || len(groups[1].Variants) != 2 {
		t.Fatalf("cga02 组错误（_blur 应被排除）: %+v", groups[1])
	}
	if groups[2].Name != "cgxb1" {
		t.Fatalf("cgxb1 组错误: %+v", groups[2])
	}
	if len(groups[2].Images) != 2 || groups[2].Images[0][0] != "cgxb1-01" {
		t.Fatalf("Images 与 Variants 应平行: %+v", groups[2].Images)
	}
	// 空表不 panic。
	if got := BuildCgGroups(nil); len(got) != 0 {
		t.Fatalf("空表应得空组，实 %d", len(got))
	}
}

func TestBuildScenes(t *testing.T) {
	pages := []ScenePage{
		{Page: 1, Records: []string{"thumb_cga01-01", "sop3", "scene_a001", "sop3", "", ""}},
		{Page: 7, Records: []string{"thumb_cgb13-01", "sb06"}},
		{Page: 10, Records: []string{"thumb_cgb11-01", "sb05", "scene_d001"}},
	}
	scenes := BuildScenes(pages)
	if len(scenes) != 3 {
		t.Fatalf("scenes = %d, want 3", len(scenes))
	}
	// 第一页完整形。
	if scenes[0].Thumb != "thumb_cga01-01" || scenes[0].Script != "sop3" || scenes[0].SceneID != "scene_a001" {
		t.Fatalf("scene[0] = %+v", scenes[0])
	}
	if scenes[0].Title != "scene_a001" || len(scenes[0].Images) != 1 || scenes[0].Images[0] != "thumb_cga01-01" {
		t.Fatalf("scene[0] Title/Images 错误: %+v", scenes[0])
	}
	if scenes[0].Index != 0 || scenes[0].Page != 1 {
		t.Fatalf("scene[0] Index/Page 错误: %+v", scenes[0])
	}
	// 只有 2 条的页：script 兜底 SceneID/Title。
	if scenes[1].SceneID != "sb06" || scenes[1].Title != "sb06" || scenes[1].Images[0] != "thumb_cgb13-01" {
		t.Fatalf("scene[1] 兜底错误: %+v", scenes[1])
	}
	if scenes[2].SceneID != "scene_d001" || scenes[2].Script != "sb05" {
		t.Fatalf("scene[2] 错误: %+v", scenes[2])
	}
	if scenes[2].Index != 2 || scenes[2].Page != 10 {
		t.Fatalf("scene[2] Index/Page 错误: %+v", scenes[2])
	}
	if got := BuildScenes(nil); len(got) != 0 {
		t.Fatalf("空页表应得空场景")
	}
}

func TestBuildMusicFiltersVoice(t *testing.T) {
	music := BuildMusic([]string{
		"bgm04.ogg", "bgm10.ogg", "bgm01.ogg", "bgm02.ogg",
		"miy_0792.ogg", "se_001.ogg", "yui_0184.ogg", // 语音/SE 不该进音乐鉴赏
		"bgm01.ogg", // 重复去重
	})
	if len(music) != 4 {
		t.Fatalf("music = %d, want 4: %+v", len(music), music)
	}
	// 排序后 bgm01 在前。
	for i, want := range []string{"bgm01", "bgm02", "bgm04", "bgm10"} {
		if music[i].Name != want || music[i].File != want+".ogg" {
			t.Fatalf("music[%d] = %+v, want %s", i, music[i], want)
		}
		if music[i].Index != i || music[i].Unlock != want || music[i].Title != want {
			t.Fatalf("music[%d] 字段错误: %+v", i, music[i])
		}
	}
}

func TestBuildScriptsPreservesOrder(t *testing.T) {
	scripts := BuildScripts(recs("sed1", "main", "startup", ""))
	if len(scripts) != 3 || scripts[1].Name != "main" || scripts[1].Index != 1 {
		t.Fatalf("scripts = %+v", scripts)
	}
}

func TestCatalogHelpers(t *testing.T) {
	src := &fakeSource{
		tables: map[string][]arc.Record{
			"list_script.tbl":  recs("sed1", "main", "startup"),
			"list_cg.tbl":      recs("black", "cga01-01", "cga01-02", "cgxb1-01"),
			"scene_page01.tbl": recs("thumb_cga01-01", "sop3", "scene_a001", "sop3"),
			"cg_page01.tbl":    recs("thumb_cga01-01", "cga01-01", "cga01-02"),
		},
		names: []string{"bgm02.ogg", "bgm01.ogg", "miy_0792.ogg"},
	}
	c := Load(src)
	if len(c.Scripts) != 3 || c.ScriptName(1) != "main" || c.ScriptIndex("startup") != 2 || c.ScriptIndex("nope") != -1 {
		t.Fatalf("Scripts helper 错误: %+v", c.Scripts)
	}
	if g := c.CgGroupOf("cga01-02"); g == nil || g.Name != "cga01" {
		t.Fatalf("CgGroupOf 错误: %+v", g)
	}
	if c.CgGroupOf("bg001a") != nil {
		t.Fatalf("非变体不应返回组")
	}
	if !c.HasCgVariant("cga01-01") || c.HasCgVariant("bg001a") {
		t.Fatalf("HasCgVariant 错误")
	}
	if !c.HasScene("scene_a001") || !c.HasScene("sop3") || c.HasScene("nope") {
		t.Fatalf("HasScene 错误: %+v", c.SceneSet)
	}
	if !c.HasMusic("bgm01") || !c.HasMusic("bgm01.ogg") || c.HasMusic("miy_0792") {
		t.Fatalf("HasMusic 错误: %+v", c.MusicSet)
	}
	// nil Catalog 不 panic。
	var nilC *Catalog
	if nilC.ScriptIndex("main") != -1 || nilC.HasMusic("bgm01") || nilC.CgGroupOf("cga01-01") != nil {
		t.Fatalf("nil Catalog helper 应返回零值")
	}
	// Load(nil) 不 panic。
	if Load(nil) == nil {
		t.Fatalf("Load(nil) 应返回空目录而非 nil")
	}
	// 缺表的 Source 只留空。
	empty := Load(&fakeSource{tables: map[string][]arc.Record{}})
	if len(empty.Scripts) != 0 || len(empty.Cg) != 0 || len(empty.Scenes) != 0 || len(empty.Music) != 0 {
		t.Fatalf("缺表应得空目录: %+v", empty)
	}
}

func TestUnlocks(t *testing.T) {
	u := NewUnlocks(nil) // 全新游戏：全部锁定
	if u.CgUnlockedByName("cga01-01") || u.SceneUnlocked(0) || u.MusicUnlocked("bgm01") {
		t.Fatalf("nil SystemData 应全部锁定")
	}
	if !u.MarkCg("cga01-01") || u.MarkCg("cga01-01") {
		t.Fatalf("MarkCg 去重错误")
	}
	if !u.CgUnlockedByName("cga01-01") || !u.CgUnlockedByName("thumb_cga01-01") {
		t.Fatalf("CgUnlockedByName 应认 thumb_ 前缀")
	}
	if c := CgKey("thumb_cga01-01"); c != "cga01-01" {
		t.Fatalf("CgKey = %q", c)
	}
	if !u.MarkScene("scene_a001") || !u.SceneUnlocked(0) || u.SceneUnlocked(1) || u.SceneUnlocked(-1) {
		t.Fatalf("Scene 解锁错误")
	}
	if !u.MarkMusic("bgm01.ogg") || !u.MusicUnlocked("bgm01") {
		t.Fatalf("Music 解锁错误")
	}
	if BgmKey("bgm10.ogg") != "bgm10" {
		t.Fatalf("BgmKey 错误")
	}
	// 写回 SystemData 再读回。
	var data save.SystemData
	u.Apply(&data)
	u2 := NewUnlocks(&data)
	if !u2.CgUnlockedByName("cga01-01") || !u2.SceneUnlocked(0) || !u2.MusicUnlocked("bgm01") {
		t.Fatalf("Apply 回写错误: %+v", data)
	}
	// Apply 到 nil 不 panic。
	u.Apply(nil)
}

func TestLayouts(t *testing.T) {
	// 标题：五按钮、顺序、资源名、pressed 变体。
	items := TitleMenu()
	if len(items) != 5 {
		t.Fatalf("TitleMenu = %d, want 5", len(items))
	}
	wantKeys := []string{"start", "load", "config", "extra", "quit"}
	wantImgs := []string{"title_start", "title_load", "title_config", "title_extra", "title_exit"}
	for i, it := range items {
		if it.Key != wantKeys[i] || it.Image != wantImgs[i] || it.ImageOn != wantImgs[i]+"-o" {
			t.Fatalf("TitleMenu[%d] = %+v", i, it)
		}
		if it.Label == "" || it.Rect.W <= 0 || it.Rect.H <= 0 {
			t.Fatalf("TitleMenu[%d] Label/Rect 为空: %+v", i, it)
		}
	}
	if TitleBG != "title_bg" || TitleBgm != "bgm01" {
		t.Fatalf("TitleBG/Bgm = %q/%q", TitleBG, TitleBgm)
	}
	// 存档/读档：10x10、银行/槽位名 Zusammensetzung、load 返回复用 save_return。
	saveL := SaveLayoutFor("save")
	loadL := SaveLayoutFor("load")
	if saveL.Banks != 10 || saveL.Slots != 10 || len(saveL.BankRects) != 10 || len(saveL.SlotRects) != 10 {
		t.Fatalf("SaveLayout 网格错误: %+v", saveL)
	}
	if saveL.Background != "save_bg" || loadL.Background != "load_bg" {
		t.Fatalf("SaveLayout 背景错误: %q/%q", saveL.Background, loadL.Background)
	}
	if SaveBankImage("save_bank", 1) != "save_bank01" || SaveSlotImage("load_slot", 10) != "load_slot10" {
		t.Fatalf("银行/槽位名组合错误")
	}
	if loadL.Return != "save_return" {
		t.Fatalf("load 返回应复用 save_return（无 load_return 条目），实 %q", loadL.Return)
	}
	// 配置/回想/extra/确认：资源名非空且与 Graphic 名一致。
	cfg := ConfigLayoutFor()
	if cfg.Background != "config_bg" || cfg.Title != "config_title" || cfg.Return != "config_return" || cfg.Pin != "config_pin" {
		t.Fatalf("ConfigLayout 资源名错误: %+v", cfg)
	}
	bl := BacklogLayoutFor()
	if bl.Overlay != "log_overlay" || bl.Down != "log_dw" || bl.Slider != "log_slider" {
		t.Fatalf("BacklogLayout 资源名错误: %+v", bl)
	}
	ex := ExtraLayoutFor()
	if ex.CgTab.Image != "cg_btn1" || ex.SceneTab.Image != "cg_btn2" || ex.MusicTab.Image != "cg_btn3" || ex.Return != "cd_return" {
		t.Fatalf("ExtraLayout 页签错误: %+v", ex)
	}
	if ex.CgBackground != "cg_bg" || ex.MusicBackground != "bgm_bg" {
		t.Fatalf("ExtraLayout 背景错误: %+v", ex)
	}
	conf := ConfirmLayoutFor()
	if conf.Background != "confirm_bg" || conf.Yes != "confirm_yes" || conf.No != "confirm_no" {
		t.Fatalf("ConfirmLayout 错误: %+v", conf)
	}
	if ConfirmMsgFor("save") != "confirm_msg_save" || ConfirmMsgFor("nope") != "" {
		t.Fatalf("ConfirmMsgFor 错误")
	}
	if len(AdvButtons()) != 11 || MessageWindowImage != "adv_msg_wnd" {
		t.Fatalf("AdvButtons/MessageWindow 错误")
	}
	for _, img := range []string{
		"title_start", "save_bg", "save_bank01", "save_slot01",
		"config_bg", "config_title", "log_overlay", "cg_bg", "cg_btn1",
		"sce_bg1", "bgm_bg", "confirm_bg", "adv_msg_wnd",
	} {
		if w, h := ActualSize(img); w <= 0 || h <= 0 {
			t.Fatalf("ActualSize(%q) 应有真实尺寸", img)
		}
	}
	if w, h := ActualSize("nope_xyz"); w != 0 || h != 0 {
		t.Fatalf("ActualSize 未知图应返回 0,0，实 %dx%d", w, h)
	}
}

func TestRealCatalog(t *testing.T) {
	dir := gameDir(t)
	set, err := arc.OpenSet(dir)
	if err != nil {
		t.Fatalf("OpenSet: %v", err)
	}
	defer set.Close()
	c := Load(set)
	// list_script 150 条，main 在 147（与提取物一致）。
	if len(c.Scripts) != 150 {
		t.Fatalf("Scripts = %d, want 150", len(c.Scripts))
	}
	if c.ScriptName(0) != "sed1" || c.ScriptIndex("main") != 147 || c.ScriptName(147) != "main" {
		t.Fatalf("Scripts 首尾/ main 下标错误: %q @%d", c.ScriptName(0), c.ScriptIndex("main"))
	}
	if _, ok := c.ScriptSet["startup"]; !ok {
		t.Fatalf("startup 不在脚本表")
	}
	// list_cg 1675 条中 CG 变体 1094 条（见 tmpwork 提取统计），
	// 分组后变体总数应与之相等；非 CG 名（black/bg*/map_*）不进分组。
	totalVariants := 0
	bad := 0
	for _, g := range c.Cg {
		totalVariants += len(g.Variants)
		for _, v := range g.Variants {
			if !IsCgVariant(v) {
				bad++
			}
		}
	}
	if totalVariants == 0 {
		t.Fatalf("CgGroups 为空")
	}
	if bad != 0 {
		t.Fatalf("CgGroups 混入非变体 %d 条", bad)
	}
	t.Logf("Cg groups=%d variants=%d first=%s/%v", len(c.Cg), totalVariants, c.Cg[0].Name, c.Cg[0].Variants[:min(3, len(c.Cg[0].Variants))])
	if c.Cg[0].Name != "cga01" || c.Cg[0].Variants[0] != "cga01-01" || c.Cg[0].Thumb != "thumb_cga01-01" {
		t.Fatalf("首组错误: %+v", c.Cg[0])
	}
	if !c.HasCgVariant("cga01-01") || c.HasCgVariant("black") || c.HasCgVariant("bg011b") {
		t.Fatalf("Cg 集合错误")
	}
	if g := c.CgGroupOf("cga01-02"); g == nil || g.Name != "cga01" {
		t.Fatalf("CgGroupOf(cga01-02) 错误")
	}
	// scene 19 页 → 19 条场景；首条 sop3/scene_a001。
	if len(c.Scenes) != 19 {
		t.Fatalf("Scenes = %d, want 19", len(c.Scenes))
	}
	if c.Scenes[0].Page != 1 || c.Scenes[0].Script != "sop3" || c.Scenes[0].SceneID != "scene_a001" || c.Scenes[0].Thumb != "thumb_cga01-01" {
		t.Fatalf("Scenes[0] = %+v", c.Scenes[0])
	}
	// cg 5 页。
	if len(c.CgPages) != 5 || c.CgPages[0].Thumb != "thumb_cga01-01" || c.CgPages[0].Variants[0] != "cga01-01" {
		t.Fatalf("CgPages = %+v", c.CgPages)
	}
	// music 10 首，bgm01 在前。
	if len(c.Music) != 10 || c.Music[0].Name != "bgm01" || c.Music[9].Name != "bgm10" {
		t.Fatalf("Music = %+v", c.Music)
	}
	if !c.HasMusic("bgm01") || c.HasMusic("miy_0792") {
		t.Fatalf("Music 集合错误")
	}
	// 标题背景实测 800x600。
	w, h, _, err := set.Image("title_bg")
	if err != nil {
		t.Fatalf("Image(title_bg): %v", err)
	}
	if w != ScreenW || h != ScreenH {
		t.Fatalf("title_bg = %dx%d, want %dx%d", w, h, ScreenW, ScreenH)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
