package session

import (
	"os"
	"path/filepath"
	"testing"
)

// 品牌 Detect 不再依赖 yui.exe：绿色版/汉化版删掉启动器也必须能被 core.Detect 找到。
func TestDetectOverdoseRealData(t *testing.T) {
	const root = "/root/code/engine/游戏/YUI"
	if _, err := os.Stat(filepath.Join(root, "優衣", "script.arc")); err != nil {
		t.Skip("未找到 YUI 游戏目录")
	}
	if !(brand{}).Detect(root) {
		t.Fatal("brand.Detect 未识别 YUI")
	}
	if !(brand{}).Detect(filepath.Join(root, "優衣")) {
		t.Fatal("brand.Detect 未识别数据目录本身")
	}
}
