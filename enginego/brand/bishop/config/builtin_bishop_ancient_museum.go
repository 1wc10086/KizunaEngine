package config

import "fmt"

// builtin_bishop_ancient_museum.go 给 Tbxdl 世代（特別病棟 旧版与 Win10）补鉴赏室布局。
//
// 这一代的鉴赏室没有 [museum phase] 的页签：CG 与场景各是整屏底图加固定 5x4 网格，
// 音乐页是底图加 22 行曲目高亮条。渲染层凭 Museum.Ancient 切到专用分支。
// 只在鉴赏布局为空时写入，不覆盖其它作品的 bsxx.cfg。
//
// 注意：导航按钮、音乐控制热区、音量轨道的坐标是按素材尺寸与原版截图估算的，
// 尚未与截图逐像素比对；曲目 BGM 名按 bgm01..bgm22 假定。

const (
	ancientMuseumGridX   = 30
	ancientMuseumGridY   = 70
	ancientMuseumColStep = 155
	ancientMuseumRowStep = 125
	ancientMuseumCell    = 120
	ancientMuseumCellH   = 90
	ancientMuseumCols    = 5
	ancientMuseumRows    = 4
	ancientMusicRows     = 22
	ancientMusicX        = 50
	ancientMusicTop      = 58
	ancientMusicRowH     = 22
	ancientMusicWidth    = 270
)

func applyBuiltinAncientMuseum(cfg *BsxxConfiguration) {
	// 已有鉴赏布局（重复调用或其它来源）时不覆盖。
	if cfg == nil || !cfg.Museum.Empty() {
		return
	}
	names := make([]string, ancientMusicRows)
	tracks := make([]UIButton, ancientMusicRows)
	for index := range names {
		names[index] = fmt.Sprintf("bgm%02d", index+1)
		// 每行是 music_select 竖排高亮图里的一截：按行裁剪，Frames=1 不切帧。
		tracks[index] = UIButton{
			Image:  "music_select",
			Frames: 1,
			X:      ancientMusicX,
			Y:      ancientMusicTop + index*ancientMusicRowH,
			Clip:   [4]int{0, index * ancientMusicRowH, ancientMusicWidth, ancientMusicRowH},
		}
	}
	// CG 与场景共用 5x4 坐标；导航按钮只在 CG 页声明，场景页绘制/命中时复用它。
	grid := ancientMuseumGridPoints()
	cfg.Museum = Museum{
		Ancient: true,
		Modes:   []MuseumMode{{Key: "music"}, {Key: "cg"}, {Key: "scene"}},
		CG: MuseumCGPage{
			Base:       "cg_base",
			Rows:       ancientMuseumRows,
			Columns:    ancientMuseumCols,
			Thumbs:     grid,
			ThumbW:     ancientMuseumCell,
			ThumbH:     ancientMuseumCellH,
			ThumbFrame: UIButton{Image: "gallery_thumbbase", Frames: 1},
			NavPrev:    ancientMuseumNav("cg_btn_prev", 656),
			NavTop:     ancientMuseumNav("cg_btn_top", 684),
			NavNext:    ancientMuseumNav("cg_btn_next", 712),
			NavBack:    ancientMuseumNav("cg_btn_siru", 740),
		},
		Scene: MuseumScenePage{
			Base:       "sc_base",
			Rows:       ancientMuseumRows,
			Columns:    ancientMuseumCols,
			Thumbs:     grid,
			ThumbW:     ancientMuseumCell,
			ThumbH:     ancientMuseumCellH,
			ThumbFrame: UIButton{Image: "gallery_thumbbase", Frames: 1},
			NavPrev:    ancientMuseumNav("cg_btn_prev", 656),
			NavTop:     ancientMuseumNav("cg_btn_top", 684),
			NavNext:    ancientMuseumNav("cg_btn_next", 712),
			NavBack:    ancientMuseumNav("cg_btn_siru", 740),
		},
		Music: MuseumMusicPage{
			Base:              "music_base",
			Play:              ancientMuseumHotspot(440, 520, 48, 40),
			Stop:              ancientMuseumHotspot(492, 520, 48, 40),
			Prev:              ancientMuseumHotspot(544, 520, 48, 40),
			Next:              ancientMuseumHotspot(596, 520, 48, 40),
			ControlBase:       UIButton{Image: "music_db", Frames: 1, X: 600, Y: 150},
			Volume:            UIButton{Image: "music_volume", Frames: 1, X: 600, Y: 150},
			VolumeKnob:        true,
			VolumeRegion:      [4]int{590, 150, 32, 200},
			VolumeTrackHeight: 188,
			Tracks:            tracks,
			Names:             names,
			TrackSelect:       UIButton{Image: "music_select", Frames: ancientMusicRows, X: ancientMusicX, Y: ancientMusicTop},
			TrackCount:        ancientMusicRows,
		},
		// 查看器的上一张/下一张/返回与 CG 页导航同图；返回即回到鉴赏格子页。
		Viewer: CGViewer{
			Prev:        ancientMuseumNav("cg_btn_prev", 656),
			Next:        ancientMuseumNav("cg_btn_next", 712),
			Exit:        ancientMuseumNav("cg_btn_siru", 740),
			ButtonBase:  0,
			HoverOffset: 1,
		},
	}
}

// ancientMuseumNav 返回一个 22x66（三帧）的导航按钮，统一贴在右上角 y=20。
func ancientMuseumNav(image string, x int) UIButton {
	return UIButton{Image: image, Frames: 3, X: x, Y: 20}
}

// ancientMuseumHotspot 返回只用于命中的热区。底图已经烘焙了控制图标，
// 绘制时不再叠加，所以 Clip 只为命中判定提供矩形。
func ancientMuseumHotspot(x, y, width, height int) UIButton {
	return UIButton{Image: "music_base", Frames: 1, X: x, Y: y, Clip: [4]int{x, y, width, height}}
}

// ancientMuseumGridPoints 按行优先生成 5x4 格子的左上角坐标，每页最多 20 格。
func ancientMuseumGridPoints() []UIPoint {
	points := make([]UIPoint, 0, ancientMuseumCols*ancientMuseumRows)
	for row := 0; row < ancientMuseumRows; row++ {
		for col := 0; col < ancientMuseumCols; col++ {
			points = append(points, UIPoint{
				X: ancientMuseumGridX + col*ancientMuseumColStep,
				Y: ancientMuseumGridY + row*ancientMuseumRowStep,
			})
		}
	}
	return points
}
