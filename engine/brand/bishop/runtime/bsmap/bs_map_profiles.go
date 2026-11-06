package bsmap

import (
	"kizuna/engine/core"
)

type BsMapProfile interface {
	Name() string
	Matches(context BsMapContext) bool
	Build(context BsMapContext) *BsMapLayout
}

func AllBsMapProfiles() []BsMapProfile {
	return orderedBsMapProfiles
}

var orderedBsMapProfiles = []BsMapProfile{
	&BsMapProfileKd{},
	&BsMapProfileMk2{},
	&BsMapProfileMk2Modern{},
	// 牝教師（MK/MKIE）必须排在 Houkago2 之前：MK 与后注册的 Houkago2
	// 同为 Rev.5、都用 map_base，而 miyu1_norm 的指纹更具体，先匹配免得
	// 被后者的通用轮廓吃掉。MKIE 是现代签名，同位置注册便于两代相邻维护。
	&BsMapProfileMkLegacy{},
	&BsMapProfileMkLegacyModern{},
	&BsMapProfileHoukago2{},
	&BsMapProfileUk{},
	&BsMapProfileKs{},
	&BsMapProfileSansha{},
	&BsMapProfileSchoolHeaven{},
	&BsMapProfileGakuen2{},
	&BsMapProfileMddvd{},
	// 学園（GK）必须在通用 MapPrms 之前：两者都读 MapPrms.dat，但 GK 的
	// 按钮在右栏、且有工具条/日期卡/头像，通用几何会把热区放到左半屏。
	&BsMapProfileGk{},
	&BsMapProfileTbxdl{},
	&BsMapProfileMapPrms{},
	// ヒメドレイ 的 map.dat 与 MK2/放課後2 同构，但脚本结果仍落到
	// tht_map/lyz_map 等标签；放在 LabelScan 之前，先补底图/工具条，
	// 再让 LabelScan 负责生成可点的角色标签。
	&BsMapProfileHimedorei{},
	&BsMapProfileLabelScan{},
}

func BuildBsMapProfiles(context BsMapContext) BsMapLayout {
	for _, profile := range orderedBsMapProfiles {
		if !profile.Matches(context) {
			continue
		}
		layout := func() (result *BsMapLayout) {
			defer func() {
				if recovered := recover(); recovered != nil {
					core.Logf("script: map profile '%s' failed: %s", profile.Name(), bsMapRecoverMessage(recovered))
					result = nil
				}
			}()
			return profile.Build(context)
		}()
		// AllowEmpty 的作品（UK/UKIE、牝教師2/MK 两代）即使当天没有
		// 可点目标也必须打开地图：底图和「時間を進める」仍然要显示，
		// 否则脚本拿上一张图的 #result 继续分派会重放已经看过的场景。
		// 旧实现只放行非空布局，AllowEmpty 永远走不到 beginMapSelect。
		if layout != nil && (!layout.IsEmpty() || layout.AllowEmpty) {
			core.Logf("script: map profile '%s' -> %d item(s) from %s", profile.Name(), len(layout.Items), layout.Source)
			return *layout
		}
	}
	return BsMapLayoutEmpty
}

func bsMapRecoverMessage(recovered any) string {
	if err, ok := recovered.(error); ok {
		return err.Error()
	}
	return "panic"
}
