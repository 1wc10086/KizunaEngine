package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	overscript "kizuna/enginego/brand/overdose/script"
)

// looseScriptDirs 是“翻译后剧本”的覆盖目录优先级。
//
// 为什么需要覆盖：原版剧本在 script.arc（加密 + zlib），汉化不可能让玩家
// 重打包 1.1GB 的归档。kernel 的 ASB codec 把单个 .asb 解成 JSON、翻译后再
// 编回 .asb；把结果按同名放进这些目录，引擎优先读散装文件，找不到才回归档。
// 目录名同时覆盖常见的 tooling 输出习惯（script/、translated/）。
var looseScriptDirs = []string{"script", "scripts", "translated", "translation"}

// loadScriptEntry 读一个脚本：散装覆盖优先，归档兜底。
//
// 两种散装形态都接受：
//   - ASB\0 容器（kernel encode 的产物，内部仍加密 + zlib）；
//   - 已解压的裸指令流（部分提取工具直接吐这种）。
//
// 散装文件存在但解析失败时直接报错，不静默回退归档：玩家以为在玩汉化版，
// 实际却跑原版，比看到一条明确的“剧本损坏”更难排查。
func (s *Session) loadScriptEntry(name string) (*overscript.Script, error) {
	base := strings.TrimSuffix(name, ".asb")
	if base == "" {
		return nil, fmt.Errorf("overdose: 空脚本名")
	}
	// 脚本名来自 TBL/ASB 字符串；禁止路径分隔符与 `..`，避免散装覆盖路径
	// 被构造脚本带出游戏目录。
	if base != filepath.Base(base) || strings.ContainsAny(base, `/\`) {
		return nil, fmt.Errorf("overdose: 非法脚本名 %q", name)
	}
	for _, sub := range looseScriptDirs {
		path := filepath.Join(s.dir, sub, base+".asb")
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		s.noteScript(base)
		if len(raw) >= 4 && string(raw[:4]) == "ASB\x00" {
			return overscript.Unmarshal(base, raw)
		}
		// 裸流没有长度头；Parse 只按指令 size 推进，能直接吃。
		return overscript.Parse(base, raw)
	}
	// 兼容直接放在游戏根目录的覆盖文件（有些汉化补丁就这么发）。
	if raw, err := os.ReadFile(filepath.Join(s.dir, base+".asb")); err == nil {
		s.noteScript(base)
		if len(raw) >= 4 && string(raw[:4]) == "ASB\x00" {
			return overscript.Unmarshal(base, raw)
		}
		return overscript.Parse(base, raw)
	}
	if s.archives == nil {
		return nil, fmt.Errorf("overdose: 归档未打开")
	}
	raw, err := s.archives.Entry(base + ".asb")
	if err != nil {
		return nil, err
	}
	s.noteScript(base)
	return overscript.Unmarshal(base, raw)
}
