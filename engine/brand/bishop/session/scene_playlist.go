package session

import (
	"encoding/json"
	"os"
	"path/filepath"

	"kizuna/engine/brand/bishop/data"
	"kizuna/engine/core"
)

// scene_playlist.go 保存场景鉴赏的"播放列表"。
//
// 原引擎允许玩家在场景页把喜欢的场景加入一个临时列表，再从播放列表
// 一次挑出来看。它不是存档槽，也不参与全局解锁，所以单独存一个 JSON，
// 放在游戏 save 目录下；不同作品互不影响。

const scenePlaylistFileName = "kizuna-playlist.json"

// scenePlaylistFile 是磁盘格式。用对象而不是裸数组，给以后加字段留余地。
type scenePlaylistFile struct {
	Labels []string `json:"labels"`
}

func (s *Session) scenePlaylistPath() string {
	return filepath.Join(dataSaveDir(s.dataDir), scenePlaylistFileName)
}

// ensureScenePlaylist 惰性读取播放列表；读不到或损坏时返回空表。
//
// 播放列表只是玩家的偏好，坏文件不该阻止鉴赏室打开，所以这里不报错。
func (s *Session) ensureScenePlaylist() []string {
	if s.playlistLoaded {
		return s.playlist
	}
	s.playlistLoaded = true
	data, err := os.ReadFile(s.scenePlaylistPath())
	if err != nil {
		return s.playlist
	}
	var file scenePlaylistFile
	if err := json.Unmarshal(data, &file); err != nil {
		core.Logf("playlist: ignore damaged file: %v", err)
		return s.playlist
	}
	seen := make(map[string]struct{}, len(file.Labels))
	for _, label := range file.Labels {
		if label == "" {
			continue
		}
		if _, exists := seen[label]; exists {
			continue
		}
		seen[label] = struct{}{}
		s.playlist = append(s.playlist, label)
	}
	return s.playlist
}

// saveScenePlaylist 原子写回播放列表。
func (s *Session) saveScenePlaylist() {
	path := s.scenePlaylistPath()
	if !core.EnsureDir(filepath.Dir(path)) {
		core.Logf("playlist: create directory failed")
		return
	}
	data, err := json.MarshalIndent(scenePlaylistFile{Labels: s.playlist}, "", "  ")
	if err != nil {
		return
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, data, 0o644); err != nil {
		core.Logf("playlist: write failed: %v", err)
		return
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		core.Logf("playlist: commit failed: %v", err)
	}
}

// addSceneToPlaylist 把场景加入播放列表；返回是否真的新增。
//
// 同一个场景重复点击只保留一条：播放列表是"挑片"列表，不是播放历史。
func (s *Session) addSceneToPlaylist(label string) bool {
	if label == "" {
		return false
	}
	s.ensureScenePlaylist()
	for _, existing := range s.playlist {
		if existing == label {
			return false
		}
	}
	// 原布局最多 10 页 × 42 格；超出后不再接受，避免文件无限增长。
	if len(s.playlist) >= 10*s.scenePlaylistSlotMax() {
		return false
	}
	s.playlist = append(s.playlist, label)
	s.saveScenePlaylist()
	return true
}

// scenePlaylistSlotMax 每页槽位数（来自 [scene playlist] slot max）。
func (s *Session) scenePlaylistSlotMax() int {
	if s.config == nil {
		return 42
	}
	slot := s.config.Museum.Scene.Playlist.SlotMax
	if slot <= 0 {
		return 42
	}
	return slot
}

// scenePlaylistPages 播放列表页数（至少 1 页，最多 10 页）。
func (s *Session) scenePlaylistPages() int {
	slot := s.scenePlaylistSlotMax()
	count := (len(s.ensureScenePlaylist()) + slot - 1) / slot
	if count < 1 {
		count = 1
	}
	if count > 10 {
		count = 10
	}
	return count
}

// scenePlaylistSlot 取某页某格的场景标签；空槽返回空串。
func (s *Session) scenePlaylistSlot(page, slot int) string {
	list := s.ensureScenePlaylist()
	index := page*s.scenePlaylistSlotMax() + slot
	if index < 0 || index >= len(list) {
		return ""
	}
	return list[index]
}

// sceneByLabel 按回放标签找场景；找不到返回零值。
func (s *Session) sceneByLabel(label string) (data.ReplayScene, bool) {
	if label == "" {
		return data.ReplayScene{}, false
	}
	catalog := s.ensureScenes()
	if catalog == nil {
		return data.ReplayScene{}, false
	}
	index := catalog.IndexOfLabel(label)
	if index < 0 || index >= len(catalog.Scenes) {
		return data.ReplayScene{}, false
	}
	return catalog.Scenes[index], true
}

// wheelScenePlaylist 在播放列表里翻页。
func (s *Session) wheelScenePlaylist(delta int32) {
	ui := s.ui
	if ui == nil {
		return
	}
	pages := s.scenePlaylistPages()
	page := clampInt(ui.playlistPage+int(delta), 0, pages-1)
	if page == ui.playlistPage {
		return
	}
	ui.playlistPage = page
	ui.playlistSelected = -1
	s.composer.Invalidate()
}
