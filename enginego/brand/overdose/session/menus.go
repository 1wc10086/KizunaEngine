// 标题菜单与画廊：Flutter 不直接读 overdose_spec 的菜单定义，
// 这里的几何是"引擎内按钮图"与"宿主通用按钮层"之间的翻译表。
//
// title_start/load/config/extra/exit 是 spec §7 的五按钮图名，
// 基础图与 -o 压图（pressed）成对出现；宿主 title_layer 只认
// key/label/rect，这里描述文件与宿主坐标的对应关系。
// 画廊三类（CG/音乐/场景）只差数据源，按钮与合成分开在 gallery.go。
package session

import (
	"kizuna/enginego/brand/overdose/arc"
	overgfx "kizuna/enginego/brand/overdose/gfx"
	"kizuna/enginego/brand/overdose/menus"
	"kizuna/enginego/core"
)

// titleMenu 是标题主界面的按钮表：只存 key 与几何，不含渲染。
// 按钮图不在这里，是因为 Compose 直接按 key 取图即可，
// 存图的中间结构只会让命中判定与渲染两处各存一遍。
type titleMenu struct {
	items []core.TitleMenuItem
	// hitRev 是菜单几何的修订号：宿主按它判断按钮是否移位，避免旧坐标误点。
	hitRev int
}

// newTitleMenu 按 sq_menu.nut 的真实 Register 坐标建表。
// 坐标/尺寸统一从 menus.TitleMenu 取，避免“引擎画一套、宿主命中另一套”：
// Flutter TitleLayer 的热区完全来自这里返回的 X/Y/W/H。
func newTitleMenu(cache *overgfx.Cache) *titleMenu {
	m := &titleMenu{}
	labels := map[string]string{
		"start":  "はじめから",
		"load":   "つづきから",
		"config": "オプション",
		"extra":  "EXTRA",
		"quit":   "終了",
	}
	for _, it := range menus.TitleMenu() {
		m.items = append(m.items, core.TitleMenuItem{
			Key:     it.Key,
			Label:   labels[it.Key],
			X:       int32(it.Rect.X),
			Y:       int32(it.Rect.Y),
			Width:   uint32(it.Rect.W),
			Height:  uint32(it.Rect.H),
			Enabled: true,
		})
	}
	return m
}

// hit 把宿主坐标落回按钮：半开区间 [x,x+w) × [y,y+h)。
// 按钮重叠不允许（当前布局物理不叠），命中歧义时取首个命中。
func (m *titleMenu) hit(x, y int32) (string, bool) {
	for _, it := range m.items {
		if x >= it.X && x < it.X+int32(it.Width) && y >= it.Y && y < it.Y+int32(it.Height) {
			return it.Key, true
		}
	}
	return "", false
}

// TitleMenu 暴露菜单项给 TitleMenuExposer。
// 覆盖层打开时返回空：Flutter 的 TitleLayer 在菜单为空时不再抢点击，
// 点击坐标才会落到引擎自绘的存/读档、设置、鉴赏界面。
func (s *Session) TitleMenu() []core.TitleMenuItem {
	if !s.atTitle || s.menu == nil || s.overlay != overlayNone {
		return nil
	}
	out := make([]core.TitleMenuItem, len(s.menu.items))
	copy(out, s.menu.items)
	return out
}

// SelectTitleItem 处理标题按钮：start 入剧本、load 开读取、config 回设置、
// extra 开画廊、exit 抛 HostRequest 交宿主关页面（引擎不直接退出进程）。
func (s *Session) SelectTitleItem(key string) (core.Step, error) {
	if !s.atTitle {
		return core.StepFaulted, nil
	}
	return s.selectTitleItem(key)
}

// selectTitleItem 是 Pointer 命中后的内部入口：即 SelectTitleItem 本体。
func (s *Session) selectTitleItem(key string) (core.Step, error) {
	switch key {
	case "start":
		// start 即 main.asb 入口：走 Start(1) 同一路径，标题态退出。
		return s.Start(1)
	case "load":
		// 读档界面完全由引擎画（save/load_bg + 10 bank × 10 slot）。
		s.openSaveOverlay(overlayLoad)
		return core.StepAwaitTitle, nil
	case "config":
		// 环境设定也是引擎界面，不再转交 Flutter 设置页。
		s.openOverlay(overlayConfig, core.StepAwaitTitle)
		return core.StepAwaitTitle, nil
	case "extra":
		// extra 先开 CG 页；音乐/场景用 cg_btn2/3 切页。
		s.openExtraOverlay(galleryCG)
		return core.StepAwaitTitle, nil
	case "quit":
		// 引擎不开浏览器/不退出进程：HostRequest 把动作交给宿主路由。
		s.hostRequest = "exit"
		s.hostRequestSerial++
		return core.StepAwaitTitle, nil
	}
	return core.StepAwaitTitle, nil
}

// loadScriptOrder 读 system.arc 的 list_script.tbl，返回脚本名序（不带 .asb）。
// 表缺失或解析失败返回空：调用方回退 main，不因一张表缺席让整个 Open 失败。
func loadScriptOrder(set *arc.Set) []string {
	if set == nil {
		return nil
	}
	recs, err := set.Table("list_script.tbl")
	if err != nil {
		return nil
	}
	var out []string
	for _, r := range recs {
		name := r.Name
		if name == "" {
			continue
		}
		// 表里记的是文件名形态（带 .asb），VM loader 的键是不带后缀的。
		for _, suf := range []string{".asb", ".ASB"} {
			if len(name) > len(suf) && name[len(name)-len(suf):] == suf {
				name = name[:len(name)-len(suf)]
				break
			}
		}
		out = append(out, name)
	}
	return out
}

// setHostRequest 的注释：SelectTitleItem 设 "exit" 后经 State 透传给宿主，
// 字段在 session.go 这里只做归属说明，代码在 SelectTitleItem 里。
