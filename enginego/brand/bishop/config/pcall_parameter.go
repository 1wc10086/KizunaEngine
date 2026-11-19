package config

import "strings"

const pcallParameterSection = "pcall parameter"

// PcallParameter 是 bsxx.cfg 的 [pcall parameter] 段（らぶえろの設定）。
//
// 原引擎在 pcall（引擎过程调用）时，把段里列出的脚本属性交给好感度报表
// 和 らぶえろメーター 画面读取。这里只保存属性引用名（例如 #favo_point.sakura），
// 真正的取值由宿主按名字经脚本的属性表查找。段缺失时全部为零值，其它作品
// 不受影响。
type PcallParameter struct {
	// LoveeroMeter 是らぶえろメーター的属性引用，例如 #loveero_meter.delta。
	LoveeroMeter string
	// FavoReport 是 8 位女主角好感度报表依次读取的属性引用，顺序即画面上的行序。
	FavoReport []string
}

func parsePcallParameter(document *IniDocument) PcallParameter {
	return PcallParameter{
		LoveeroMeter: strings.TrimSpace(document.Get(pcallParameterSection, "loveero_meter", "")),
		FavoReport:   splitPcallList(document.Get(pcallParameterSection, "favo_report", "")),
	}
}

// splitPcallList 按逗号切分属性列表并去掉空项。
//
// 不复用 ParsePositions：那个函数面向整数坐标，遇到 #favo_point.sakura 这种
// 带点的名字会静默丢弃。
func splitPcallList(value string) []string {
	var names []string
	for _, item := range strings.Split(value, ",") {
		if name := strings.TrimSpace(item); name != "" {
			names = append(names, name)
		}
	}
	return names
}
