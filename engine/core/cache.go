package core

import (
	"crypto/md5"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

// GameCacheDir 把"缓存根目录"变成"这个游戏专属的缓存目录"。
//
// 旧实现把每个游戏的音频物化文件、engine.log 直接铺在 cache/kizuna 下：换一个游戏
// 打开时，同名音频（bgm01、01_00001_001…）会互相顶掉，缓存无法判断归属，也没法在
// 卸载游戏后整目录清理。
//
// 这里用游戏目录的绝对路径算 md5 作为命名空间：同一个游戏无论重开多少次都落到同一个
// 目录，不同游戏互不干扰。用路径而不是脚本内容做哈希，是因为路径是零成本的稳定标识；
// 用内容哈希会强迫每次打开先读几百 MB 的 bsxx.dat。
//
// 空 cacheRoot 表示调用方不需要缓存目录，原样返回空串。
func GameCacheDir(cacheRoot, gameDir string) string {
	if strings.TrimSpace(cacheRoot) == "" {
		return ""
	}
	cleaned := filepath.Clean(gameDir)
	// 解析符号链接与相对路径，保证同一个目录无论从哪条路径进来都得到同一个 md5。
	// 两者失败都只是保持原值，不影响可用性。
	if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
		cleaned = resolved
	}
	if abs, err := filepath.Abs(cleaned); err == nil {
		cleaned = abs
	}
	sum := md5.Sum([]byte(strings.ToLower(cleaned)))
	return filepath.Join(cacheRoot, hex.EncodeToString(sum[:]))
}

// EnsureDir 建目录并返回是否可用；缓存目录建不出来时调用方应退回"不缓存"，
// 而不是让整个打开流程失败。
func EnsureDir(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	return os.MkdirAll(path, 0o755) == nil
}
