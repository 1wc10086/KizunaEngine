package state

import (
	"kizuna/engine/brand/bishop/resource"
)

const GraphicsCategory = "graphics"

type ResourceLocation interface {
	Read() []byte
	ReadPrefix(length int) []byte
	// ReadRange 按偏移读取一段字节；逐帧动画的容器太大，只能按帧取。
	ReadRange(offset, length int64) []byte
	Size() int64
	Name() string
}

type ResourceIndex interface {
	ContainsExact(name string) bool
	TryGet(name string) (ResourceLocation, bool)
	ArchivesInCategory(category string) []resource.Archive
}
