package bdt

import (
	"os"
	"path/filepath"
	"strings"

	"kizuna/engine/core"
)

// bdt_save_root.go 决定 BDT 游戏把存档写到哪里。
//
// 光盘游戏打开时会先解包到应用缓存（cache/disc/<hash>），如果直接把存档
// 放在挂载目录里，系统清理缓存时玩家的进度会一起消失。为此：
//  1. 挂载目录里有 .kizuna-disc.json，记录原始镜像的绝对路径；
//  2. 用镜像所在目录（用户自己的游戏库目录）下的 save/ 作为存档根；
//  3. 目录不可写（只读外置存储/SAF）时退回挂载目录，至少保证能玩。
func bdtSaveDirectory(dir string) string {
	manifest := filepath.Join(dir, ".kizuna-disc.json")
	if data, err := os.ReadFile(manifest); err == nil {
		for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			original := filepath.Dir(line)
			candidate := filepath.Join(original, "save")
			if directoryWritable(candidate) {
				core.Logf("bdt: save directory '%s' (game folder)", candidate)
				return candidate
			}
			break
		}
	}
	return filepath.Join(dir, "save")
}

func directoryWritable(directory string) bool {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return false
	}
	probe := filepath.Join(directory, ".kizuna-write-test")
	file, err := os.OpenFile(probe, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return false
	}
	_ = file.Close()
	_ = os.Remove(probe)
	return true
}
