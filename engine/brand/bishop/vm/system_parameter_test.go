package vm

import (
	"encoding/binary"
	"testing"
)

// 系统参数表是内联在头部的：经典布局 count@0x24 / 表@0x94，
// 现代布局 count@0x28 / 表@0xA8。旧实现把表头值当指针，OH 的 0x94
// 实际是第一条记录，整表被判空，[status2 screen] register=10..13
// 无法解析成 #miya_karte 等属性。
func TestReadSystemParametersInlineClassic(t *testing.T) {
	data := make([]byte, 0x100)
	binary.LittleEndian.PutUint32(data[0x24:], 2)
	writeSystemParameter(data[0x94:], 10, revPropertyKind, 3)
	writeSystemParameter(data[0x94+scriptSystemParameterSize:], 11, revPropertyKind, 4)
	script := &BsxScriptFile{
		data:       data,
		properties: scriptTable{count: 10},
		variables:  scriptTable{count: 4},
	}
	parameters := script.readSystemParameters(LayoutClassicValue)
	if len(parameters) != 2 {
		t.Fatalf("parameters = %+v", parameters)
	}
	if parameters[0].Key != 10 || parameters[0].Index != 3 {
		t.Fatalf("first = %+v", parameters[0])
	}
	if isProperty, slot, ok := script.ResolveSlot(11); !ok || !isProperty || slot != 4 {
		t.Fatalf("ResolveSlot(11) = %v %d %v", isProperty, slot, ok)
	}
}

func TestReadSystemParametersInlineModern(t *testing.T) {
	data := make([]byte, 0x110)
	binary.LittleEndian.PutUint32(data[0x28:], 1)
	writeSystemParameter(data[0xA8:], 10, revPropertyKind, 3)
	script := &BsxScriptFile{
		data:       data,
		properties: scriptTable{count: 10},
		variables:  scriptTable{count: 4},
	}
	parameters := script.readSystemParameters(LayoutModernValue)
	if len(parameters) != 1 || parameters[0].Key != 10 || parameters[0].Index != 3 {
		t.Fatalf("parameters = %+v", parameters)
	}
}

// 表头值确实是偏移时（早期 BSScript Rev. 的写法），内联解析会因
// kind/index 不合理而失败，随后回退到指针。
func TestReadSystemParametersPointerFallback(t *testing.T) {
	data := make([]byte, 0x100)
	binary.LittleEndian.PutUint32(data[0x24:], 1)
	// 把内联位置填成会让解析失败的随机 kind，再把表头写成指针。
	for offset := 0x94; offset < 0xB0; offset++ {
		data[offset] = 0xEE
	}
	binary.LittleEndian.PutUint32(data[0x94:], 0xB0)
	writeSystemParameter(data[0xB0:], 100, revPropertyKind, 2)
	script := &BsxScriptFile{
		data:       data,
		properties: scriptTable{count: 10},
		variables:  scriptTable{count: 4},
	}
	parameters := script.readSystemParameters(LayoutClassicValue)
	if len(parameters) != 1 || parameters[0].Key != 100 || parameters[0].Index != 2 {
		t.Fatalf("parameters = %+v", parameters)
	}
}

func writeSystemParameter(target []byte, key, kind, index int) {
	binary.LittleEndian.PutUint16(target[0:], uint16(key))
	binary.LittleEndian.PutUint16(target[2:], uint16(kind))
	binary.LittleEndian.PutUint32(target[4:], uint32(index))
}
