package session

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/core"
)

// player_name.go 决定剧本里 \A（姓）/ \B（名）占位符的替换值。
//
// 姓名有两个来源，优先级从高到低：
//  1. 作品目录里的 Startup.ini [NAME]：这是原启动器写入的玩家设定，
//     发行版常常带着它（黒の教室 的 data/Startup.ini 就是 First=上原 Last=大輔）。
//  2. knownPlayerNames 里的硬编码表：给没有 Startup.ini 的作品兜底。
//
// 两者都拿不到时返回空字符串，文本层会把占位符剥掉，至少不会把 \A/\B
// 原样画进对话框。
const (
	startupIniName    = "Startup.ini"
	startupIniSection = "NAME"

	// BISHOP 启动器的 First/Last 是"第几个名字字段"的位置语义，不是西式的
	// given/family：请按 \A=First、\B=Last 读取。黒の教室 的 First=上原（姓）、
	// Last=大輔（名），与脚本里「\A老师」「\B猛地」的用法一致。
	startupIniFirstName = "First"
	startupIniLastName  = "Last"

	maxPlayerNameRunes = 16
)

// knownPlayerNames 是已适配作品的主人公默认姓名（姓, 名）。
//
// 匹配方式是在作品目录路径里找 match 子串。保留这张表是为了兼容那些
// 不带 Startup.ini 的发行形态（侵蝕DL 的姓名内嵌在 startup.exe 里，
// 淫獄の放課後2 则写在 bsxx.ini 里）。
var knownPlayerNames = []struct {
	match  string
	family string
	given  string
}{
	{match: "侵蝕", family: "沢野", given: "雅也"},
	{match: "淫獄の放課後2", family: "野島", given: "良樹"},
	{match: "隷従の制服", family: "三原", given: "隆也"},
	// 目录名是「超狂指導日誌-学園統制録」，用作品名前半段匹配，避免全角/半角
	// 连接符差异（有的解压目录会把 "-" 写成 "～" 或全角）。
	{match: "超狂指導日誌", family: "上原", given: "弘人"},
	// 黒の教室 モーションプラス；同时兼容把「黒」写成简体「黑」的目录名。
	{match: "黒の教室", family: "上原", given: "大輔"},
	{match: "黑の教室", family: "上原", given: "大輔"},
	// 屈辱（2015）。剧本 msg#618「\A家の男に生まれていながら、なぜ\Bは…」
	// 证实 \A=姓、\B=名；msg#177 的『\A\B』就是「人体掌握能力」的持有者
	// 舘岡雄吾——屈辱2 的同一主人公，屈辱1 是他滥用能力的前日谭。
	{match: "屈辱", family: "舘岡", given: "雄吾"},
	// 課外授業 -私立白麗女子学園-（内部目录/品番 KJ）。原作支持在设置里改
	// 主人公姓名，发行目录里没有默认值；这里取官方式本与官网设定的通用名：
	// 桃香（天宮桃香）是主人公同父异母的妹妹，官方式本通篇只叫「博人」，
	// 姓随妹妹取「天宮」。脚本里 \A 用作「\A先生」、\A\B 连用表示全名，与
	// 姓+名的拆分一致。
	{match: "課外授業", family: "天宮", given: "博人"},
	// 教育指導 / 教育指導 IE。手册写明主人公默认名是「乾 義之」，
	// 玩家可在启动器里改名；发行目录通常叫 KS / KSIE，没有 Startup.ini。
	{match: "教育指導", family: "乾", given: "義之"},
}

func defaultPlayerName(title string) (string, string) {
	// 教育指導 的发行目录名是 KS / KSIE（不是作品名），表格按路径子串
	// 匹配找不到；这两个目录名足够独特，先按 basename 精确匹配一次。
	base := strings.ToLower(filepath.Base(title))
	if base == "ks" || base == "ksie" {
		return "乾", "義之"
	}
	for _, entry := range knownPlayerNames {
		if strings.Contains(title, entry.match) {
			return entry.family, entry.given
		}
	}

	return "", ""
}

// resolvePlayerName 先读原启动器的设定，再回退到硬编码表。
//
// dataDir 是真正放 bsxx.dat 的目录（本作是 data/），Startup.ini 通常就在
// 这里；某些发行版把它放在上一级（root/Files），所以游戏根目录 dir 也查一遍。
// 注意不能查 filepath.Dir(dataDir)：当 dataDir 就是游戏根时，那是整个游戏库，
// 会把别的作品的 Startup.ini 读进来。
// 传完整路径而不是 filepath.Base(dir)：玩家可能选到作品的 game 子目录。
func resolvePlayerName(dir, dataDir string) (string, string) {
	if family, given, ok := startupPlayerName(dir, dataDir); ok {
		core.Logf("session: player name from %s = %q %q", startupIniName, family, given)
		return family, given
	}

	family, given := defaultPlayerName(dir)
	if family != "" || given != "" {
		core.Logf("session: player name from built-in table = %q %q", family, given)
	}

	return family, given
}

func startupPlayerName(dir, dataDir string) (string, string, bool) {
	for _, root := range uniqueRoots(dataDir, dir) {
		path := findFile(root, startupIniName)
		if path == "" {
			continue
		}
		document, err := loadStartupIni(path)
		if err != nil {
			core.Logf("session: %s unreadable: %v", path, err)
			continue
		}
		family := strings.TrimSpace(document.Get(startupIniSection, startupIniFirstName, ""))
		given := strings.TrimSpace(document.Get(startupIniSection, startupIniLastName, ""))
		if validPlayerNamePart(family) && validPlayerNamePart(given) {
			return family, given, true
		}
		core.Logf("session: %s has no usable [%s] First/Last", path, startupIniSection)
	}

	return "", "", false
}

// loadStartupIni 处理启动器 INI 的两种实际编码：
// 带 BOM 的 UTF-16（config.ParseIniDocument 自带支持）和不带 BOM 的 UTF-8。
// 后者必须显式识别，否则按 CP932 解码会得到乱码姓名并显示到名字框里。
func loadStartupIni(path string) (*config.IniDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if !hasUTF16BOM(data) && utf8.Valid(data) && !isASCII(data) {
		return config.ParseIniDocumentText(string(data)), nil
	}
	return config.ParseIniDocument(data), nil
}

func hasUTF16BOM(data []byte) bool {
	return len(data) >= 2 &&
		((data[0] == 0xFF && data[1] == 0xFE) || (data[0] == 0xFE && data[1] == 0xFF))
}

func isASCII(data []byte) bool {
	for _, value := range data {
		if value >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// validPlayerNamePart 过滤空值、控制字符和乱码替换符。
// 姓名会直接写进对话框与角色名，不能带 \ 之类的控制码前缀。
func validPlayerNamePart(value string) bool {
	if value == "" || !utf8.ValidString(value) || len([]rune(value)) > maxPlayerNameRunes {
		return false
	}
	for _, character := range value {
		if character == utf8.RuneError || character == '\\' || unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func uniqueRoots(values ...string) []string {
	roots := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || value == "." {
			continue
		}
		duplicate := false
		for _, existing := range roots {
			if existing == value {
				duplicate = true
				break
			}
		}
		if !duplicate {
			roots = append(roots, value)
		}
	}
	return roots
}
