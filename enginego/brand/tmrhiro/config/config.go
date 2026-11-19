// Package config 解析 TMR-Hiro 的文本配置（srp.pac 里的 _system/_var/…）。
//
// 这些条目不是 .sct 容器，而是“逐行长度前缀”的文本：
//
//	u16 lineLen; line bytes(CP932)
//
// 内容形如 ini：`[SECTION]` 起段，`KEY=VALUE` 一行。_system 是引擎全局设置
// （窗口尺寸、对话框坐标、字体、音量默认值、按钮布局），_var 是鉴赏/CG
// 目录名表，_charaview 是立绘查看器的坐标与 BG 名字表，_charafilemacro /
// _filemacro 是“宏名 → 部件文件名”的映射表。
//
// 这一层只负责“把文本变成 map”，不解释业务含义：业务含义随作品变化，
// 由 session 在用到时按名字取。
package config

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"

	"kizuna/enginego/core/textcodec"
)

// Ini 是解析后的分节文本。
type Ini struct {
	Sections map[string]map[string]string
	// Order 保留节内键的原始顺序（鉴赏目录、按钮列表都按顺序枚举）。
	Order map[string][]string
}

// Get 取值，不存在返回空串。
func (i *Ini) Get(section, key string) string {
	if i == nil {
		return ""
	}
	return i.Sections[section][key]
}

// Has 报告键是否存在。
func (i *Ini) Has(section, key string) bool {
	if i == nil {
		return false
	}
	_, ok := i.Sections[section][key]
	return ok
}

// Keys 返回某节的所有键（原始顺序）。
func (i *Ini) Keys(section string) []string {
	if i == nil {
		return nil
	}
	return i.Order[section]
}

// Int 取整数（缺省值用 def）。
func (i *Ini) Int(section, key string, def int) int {
	value := i.Get(section, key)
	if value == "" {
		return def
	}
	if n, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
		return n
	}
	return def
}

// Float 取浮点（缺省值用 def）。
func (i *Ini) Float(section, key string, def float64) float64 {
	value := i.Get(section, key)
	if value == "" {
		return def
	}
	if n, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
		return n
	}
	return def
}

// Ints 取逗号分隔的整数列表。
func (i *Ini) Ints(section, key string) []int {
	value := i.Get(section, key)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			out = append(out, 0)
			continue
		}
		if n, err := strconv.Atoi(part); err == nil {
			out = append(out, n)
		} else {
			out = append(out, 0)
		}
	}
	return out
}

// Strings 取逗号分隔的字符串列表（原样保留空白）。
func (i *Ini) Strings(section, key string) []string {
	value := i.Get(section, key)
	if value == "" {
		return nil
	}
	return strings.Split(value, ",")
}

// IsTextConfig 判断条目是否是“长度前缀行”文本配置。
//
// 判据是结构性的：u16 行数不对，而是“逐行 u16 长度恰好铺满文件”。
// 所以只要最后一条能对齐到文件尾，就是这类文本；_var 这类几百行的表也能判对。
func IsTextConfig(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	pos := 0
	for pos < len(data) {
		if pos+2 > len(data) {
			return false
		}
		ln := int(binary.LittleEndian.Uint16(data[pos : pos+2]))
		pos += 2
		if pos+ln > len(data) || ln == 0 {
			return false
		}
		pos += ln
	}
	return pos == len(data)
}

// ParseIni 把“长度前缀行”文本解析成分节 map。
func ParseIni(data []byte) (*Ini, error) {
	if !IsTextConfig(data) {
		return nil, fmt.Errorf("tmrhiro/config: 不是文本配置")
	}
	ini := &Ini{Sections: make(map[string]map[string]string), Order: make(map[string][]string)}
	section := ""
	pos := 0
	for pos < len(data) {
		ln := int(binary.LittleEndian.Uint16(data[pos : pos+2]))
		pos += 2
		line := coreText(data[pos : pos+ln])
		pos += ln
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			if _, ok := ini.Sections[section]; !ok {
				ini.Sections[section] = make(map[string]string)
			}
			continue
		}
		index := strings.Index(line, "=")
		if index <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:index])
		value := line[index+1:]
		if section == "" {
			// 没有分节的行（如 _var 的裸列表）归到 "" 节，按序即可。
			if _, ok := ini.Sections[""]; !ok {
				ini.Sections[""] = make(map[string]string)
			}
		}
		ini.Sections[section][key] = value
		ini.Order[section] = append(ini.Order[section], key)
	}
	return ini, nil
}

// ParseList 把“每行一个值”的表解析成字符串列表（_var、_vulgarword）。
func ParseList(data []byte) ([]string, error) {
	if !IsTextConfig(data) {
		return nil, fmt.Errorf("tmrhiro/config: 不是文本列表")
	}
	out := make([]string, 0, 256)
	pos := 0
	for pos < len(data) {
		ln := int(binary.LittleEndian.Uint16(data[pos : pos+2]))
		pos += 2
		out = append(out, coreText(data[pos:pos+ln]))
		pos += ln
	}
	return out, nil
}

// ParseMacro 解析“宏名=部件路径”表（_filemacro / _charafilemacro）。
//
// 值里的部件用 '/' 分隔：`fm_A01_0002=A01_00_00-00_a/A01_00_00-01_b`
// 表示这张合成图由两张 GRD 叠成。空值的宏表示“没有图”，保留在表里。
func ParseMacro(data []byte) (map[string][]string, error) {
	lines, err := ParseList(data)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]string, len(lines))
	for _, line := range lines {
		index := strings.Index(line, "=")
		if index <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:index])
		value := strings.TrimSpace(line[index+1:])
		if key == "" {
			continue
		}
		parts := make([]string, 0, 4)
		for _, part := range strings.Split(value, "/") {
			part = strings.TrimSpace(part)
			// '-' / '_-' 是“该层留空”的占位符（部件名末尾带 '-suffix' 表示
			// 该层没有差分），原版按“没有部件”处理。
			if part == "" || part == "-" || strings.HasSuffix(part, "_-") {
				continue
			}
			parts = append(parts, part)
		}
		out[key] = parts
	}
	return out, nil
}

// coreText 解 CP932；行里的裸 ASCII 也走同一路径。
func coreText(raw []byte) string {
	return textcodec.Decode("cp932", raw)
}
