package bsmap

type BsMapItem struct {
	Image        string
	Frame        int
	DrawX        int
	DrawY        int
	HitX         int
	HitY         int
	HitWidth     int
	HitHeight    int
	Label        string
	Property     string
	PropertyKey  int
	Value        int
	Property2    string
	Property2Key int
	Value2       int
	Branches     []BsTbxMapBranch
	// CellHeight 是竖排装饰条的帧高（map_face 86、map_date 27、mw_date 32）。
	// BSG 头里的 FrameCount 不可靠（学園2 的条图都写 1），渲染层必须靠它
	// 才知道该从第几行裁一帧；零值保持原来的整图绘制。
	CellHeight int
}

func NewBsMapItem(image string, frame, drawX, drawY, hitX, hitY, hitWidth, hitHeight int, label string) BsMapItem {
	return BsMapItem{
		Image: image, Frame: frame, DrawX: drawX, DrawY: drawY,
		HitX: hitX, HitY: hitY, HitWidth: hitWidth, HitHeight: hitHeight,
		Label: label, PropertyKey: -1, Property2Key: -1,
	}
}

func NewBsMapItemDecor(image string, drawX, drawY, hitX, hitY int) BsMapItem {
	return NewBsMapItem(image, 0, drawX, drawY, hitX, hitY, 0, 0, "")
}

func NewBsMapItemWithProperty(image string, frame, drawX, drawY, hitX, hitY, hitWidth, hitHeight int, label, property string, propertyKey, value int) BsMapItem {
	item := NewBsMapItem(image, frame, drawX, drawY, hitX, hitY, hitWidth, hitHeight, label)
	item.Property = property
	item.PropertyKey = propertyKey
	item.Value = value
	return item
}

func NewBsMapItemFull(image string, frame, drawX, drawY, hitX, hitY, hitWidth, hitHeight int, label, property string, propertyKey, value int, property2 string, property2Key, value2 int, branches []BsTbxMapBranch) BsMapItem {
	item := NewBsMapItemWithProperty(image, frame, drawX, drawY, hitX, hitY, hitWidth, hitHeight, label, property, propertyKey, value)
	item.Property2 = property2
	item.Property2Key = property2Key
	item.Value2 = value2
	item.Branches = branches
	return item
}

func (item BsMapItem) HasLabel() bool {
	return len(item.Label) > 0
}

func (item BsMapItem) HasProperty() bool {
	return len(item.Property) > 0 || item.PropertyKey >= 0
}

func (item BsMapItem) HasSecondProperty() bool {
	return len(item.Property2) > 0 || item.Property2Key >= 0
}

func (item BsMapItem) HasBranches() bool {
	return len(item.Branches) > 0
}
