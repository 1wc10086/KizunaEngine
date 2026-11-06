package bsmap

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"kizuna/engine/brand/bishop/resource"
	"kizuna/engine/brand/bishop/vm/bs"
)

// ukFixtureDat 构造一份与 UK/UKIE map.dat 同构的测试数据并序列化：
// 2 个时段、2 张图像（null 占位 + map_f1）、2 个地点、
// 1 张解锁表（lv>=0 / lv>=5 两行）。
// 打包值语义与实机一致：低字节 = 地点号（0 = 不在图上），高字节 = 图像下标。
func ukFixtureDat(t *testing.T) []byte {
	t.Helper()
	out := []byte("BSX MAP INFO r3\x00")
	out = append(out, 't', 0) // 时间属性 #t
	out = append(out, 2)
	value := make([]byte, 4)
	binary.LittleEndian.PutUint32(value, 1)
	out = append(out, value...)
	binary.LittleEndian.PutUint32(value, 2)
	out = append(out, value...)
	out = append(out, 2) // 图像表
	out = append(out, 'n', 'u', 'l', 'l', 0)
	out = append(out, 'm', 'a', 'p', '_', 'f', '1', 0)
	out = append(out, 2) // 2 个地点
	// place 1: 图像 2，基准 (10, 20)，1 个 chunk (5, 6)
	out = append(out, 2)
	out = append(out, 10, 0, 20, 0)
	out = append(out, 1)
	binary.LittleEndian.PutUint32(value, 5)
	out = append(out, value...)
	binary.LittleEndian.PutUint32(value, 6)
	out = append(out, value...)
	// place 2: 图像 2，基准 (30, 40)，1 个 chunk (7, 8)
	out = append(out, 2)
	out = append(out, 30, 0, 40, 0)
	out = append(out, 1)
	binary.LittleEndian.PutUint32(value, 7)
	out = append(out, value...)
	binary.LittleEndian.PutUint32(value, 8)
	out = append(out, value...)
	out = append(out, 1) // 1 张解锁表
	out = append(out, 'l', 'v', 0)
	out = append(out, 'm', 'p', 0)
	out = append(out, 2) // 2 行
	// 行 lv>=0：slot0 = place1 + 图像2 (0x0201)，slot1 = place2 + 图像2 (0x0202)
	binary.LittleEndian.PutUint32(value, 0)
	out = append(out, value...)
	out = append(out, 2)
	out = append(out, 0x01, 0x02, 0x02, 0x02)
	// 行 lv>=5：slot0 = place2 + 图像2，slot1 = place1 + 图像2
	binary.LittleEndian.PutUint32(value, 5)
	out = append(out, value...)
	out = append(out, 2)
	out = append(out, 0x02, 0x02, 0x01, 0x02)
	out = append(out, 1) // 结果属性
	out = append(out, 'r', 'e', 's', 0)
	var data BsMapData
	if !TryParseBsMapData(out, &data) {
		t.Fatal("fixture map.dat rejected")
	}
	return out
}

// ukProperties 是 BsPropertyView 的最小实现：lv / mp / t 三个属性。
type ukProperties struct {
	lv    int
	state int
	time  int
}

func (p *ukProperties) ByName(name string) int {
	switch name {
	case "lv":
		return p.lv
	case "mp":
		return p.state
	case "t":
		return p.time
	}
	return -1
}

func (p *ukProperties) ByKey(int) int { return -1 }

// ukContext 建临时目录，写入 map.dat 与五张松散占位文件
// （map_base/map_hit/map_f1/map_f100），返回可直接喂给 profile 的上下文。
//
// HasDialect=false：UK/UKIE 是 BSXScript 2.0/3.1 的现代签名，BsDialect
// 没有真实判定结果；Uk profile 只服务这种现代签名 + map.dat 的组合。
func ukContext(t *testing.T, lv int, state int, timeValue int) BsMapContext {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, BsMapDataFileName), ukFixtureDat(t), 0o644); err != nil {
		t.Fatal(err)
	}
	// 松散资源用真实小文件：resource.Build 会把根目录下的文件按名登记。
	// Exists 只看索引，不需要可解码的内容。map_f100 是 Uk profile 的
	// 素材指纹，map_f1 是 fixture 里表项引用的头像，两者都要登记。
	for _, name := range []string{"map_base", "map_hit", "map_f1", "map_f100"} {
		if err := os.WriteFile(filepath.Join(root, name+".dat"), []byte{1}, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	res := resource.Build(root)
	context := BsMapContext{
		Resources:  res,
		HasDialect: false,
		GameRoot:   root,
		Properties: &ukProperties{lv: lv, state: state, time: timeValue},
	}
	return context
}

func TestBsMapProfileUkMatches(t *testing.T) {
	context := ukContext(t, 0, 0, 1)
	profile := &BsMapProfileUk{}
	if !profile.Matches(context) {
		t.Fatal("Uk profile should match a modern-signature game with map.dat + map_hit + map_f100")
	}
	// Rev/Ancient 方言（HasDialect=true）是其它 profile 的领地，UK 不碰。
	context.HasDialect = true
	context.Dialect = bs.BsScriptRev2
	if profile.Matches(context) {
		t.Fatal("Uk profile must reject Rev dialects (they belong to Rev profiles)")
	}
	context.HasDialect = false
	// 缺 map_f100 指纹时不匹配（Kd/Mk2 等其它有 map.dat 的作品）。
	// ukContext 注册的是独立文件；这里直接构造一个没有指纹的索引验证。
	context.Resources = resource.Build(t.TempDir())
	if profile.Matches(context) {
		t.Fatal("Uk profile must reject games without the map_f100 fingerprint")
	}
}

func TestBsMapProfileUkBuildWritesTableValue(t *testing.T) {
	context := ukContext(t, 0, 0, 1)
	layout := (&BsMapProfileUk{}).Build(context)
	if layout == nil || layout.IsEmpty() {
		t.Fatal("layout is empty")
	}
	if layout.Background != "map_base" || layout.Source != BsMapDataFileName {
		t.Fatalf("layout = %+v", layout)
	}
	// lv=0 行：slot0 = place1 + map_f1；#result 应写表序号 1。
	if len(layout.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(layout.Items))
	}
	item := layout.Items[0]
	if item.Property != "res" || item.Value != 1 {
		t.Fatalf("item property = %q value = %d", item.Property, item.Value)
	}
	if item.Image != "map_f1" {
		t.Fatalf("image = %q", item.Image)
	}
	// 热区 = place.BaseX + chunk.X / BaseY + chunk.Y。
	if item.HitX != 15 || item.HitY != 26 {
		t.Fatalf("hit = %d,%d", item.HitX, item.HitY)
	}
}

func TestBsMapProfileUkBuildSelectsRowByProgress(t *testing.T) {
	context := ukContext(t, 5, 0, 1)
	layout := (&BsMapProfileUk{}).Build(context)
	if layout == nil || len(layout.Items) != 1 {
		t.Fatalf("items = %+v", layout)
	}
	// lv>=5 行 slot0 = place2 → 热区应在 (37, 48)。
	if layout.Items[0].HitX != 37 || layout.Items[0].HitY != 48 {
		t.Fatalf("hit = %d,%d", layout.Items[0].HitX, layout.Items[0].HitY)
	}
}

func TestBsMapProfileUkBuildKeepsEmptyMapOpen(t *testing.T) {
	// 所有表的 state 都是 1（本时段已用过）：没有任何角色出门。原版仍显示
	// 底图与「時間を進める」；AllowEmpty 必须保持为真，否则 Session 会跳过
	// SELECT_TAIL，剧本拿着上一张图的 #result 继续分派。
	context := ukContext(t, 0, 1, 1)
	layout := (&BsMapProfileUk{}).Build(context)
	if layout == nil {
		t.Fatal("layout must not be nil")
	}
	if !layout.IsEmpty() {
		t.Fatalf("items = %d, want 0", len(layout.Items))
	}
	if !layout.AllowEmpty {
		t.Fatal("empty UK map must stay open (AllowEmpty)")
	}
	if layout.ResultProperty != "res" {
		t.Fatalf("result property = %q", layout.ResultProperty)
	}
}

func TestBsMapUkMaskUsesPlaceMinusOne(t *testing.T) {
	mask := &BsMapMaskData{ItemForValue: make([]int, 256)}
	// 下标 0 不用；地点 1 → 第 5 条热区，地点 2 → 第 9 条热区，地点 3 无热区。
	first := []int{-1, 5, 9, -1}
	reindexBsMapUkMask(mask, first)
	if mask.ItemForValue[0] != 5 || mask.ItemForValue[1] != 9 {
		t.Fatalf("map_hit value 0/1 -> items %d/%d, want 5/9", mask.ItemForValue[0], mask.ItemForValue[1])
	}
	if mask.ItemForValue[2] != -1 || mask.ItemForValue[255] != -1 {
		t.Fatalf("unmapped values -> items %d/%d, want -1/-1", mask.ItemForValue[2], mask.ItemForValue[255])
	}
}

func TestBsMapUkFaceCellFromHeader(t *testing.T) {
	// 最小 BSG 头：签名 + 宽高字段（0x16 宽、0x18 高）。两帧竖排 56x112 → 一帧 56x56。
	header := make([]byte, 0x40)
	copy(header, "BSS-Graphics")
	binary.LittleEndian.PutUint16(header[0x16:], 56)
	binary.LittleEndian.PutUint16(header[0x18:], 112)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "map_face.dat"), header, 0o644); err != nil {
		t.Fatal(err)
	}
	context := BsMapContext{Resources: resource.Build(root)}
	width, cell := bsMapUkFaceCell(context, "map_face")
	if width != 56 || cell != 56 {
		t.Fatalf("bsMapUkFaceCell = %d,%d, want 56,56", width, cell)
	}
}
