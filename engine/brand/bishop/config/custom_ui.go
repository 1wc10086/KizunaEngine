package config

// custom_ui.go 解析 define.bsa 里的"自定义窗口"定义（work/*.txt 等）。
//
// 这类文件不是图片，也不是 bsxx.cfg：原引擎在剧本发出
// IMAGE_EX type=176 时，按名字去 define.bsa 找同名 .txt，把它当成一张
// 可交互的界面来画。文件语法和 system_config_ex.layout / museum_buex_layout
// 相同（无引号键、可算式、分号注释），所以复用同一个解析器。
//
// 数据分两部分：
//   - objects：静态/按脚本属性取帧的图层（体力/好感度条、工作进度条、
//     "Down..." 覆盖层）。condition_state 是取帧的属性名，max_state 是帧数；
//     opacity 是属性名（0..255），valid_conditions 决定这一层画不画。
//   - buttons：单选组。result 是写回的属性名（#chN_select_work），
//     objects 里每个按钮有图片、坐标、值和禁用条件。
//
// 这里只做"搬运"，不求值：条件表达式依赖脚本属性表，求值放在 session。

// CustomUIDefinition 是一个 type=176 窗口的完整定义。
type CustomUIDefinition struct {
	Objects []CustomUIObject
	Groups  []CustomUIGroup
	// DisableCondition 是顶层 conditions.disable（work_start.txt 用它
	// 在四人未选齐工作时禁用开始按钮）。
	DisableCondition string
	// ReturnValue 是顶层 return_value（按钮点击时写回的值）。
	ReturnValue int
}

// CustomUIObject 是一个图层。
type CustomUIObject struct {
	Image string
	X, Y  int
	// MaxState 是竖排图的帧数；0 表示单帧。
	MaxState int
	// ConditionState 是取帧的属性名（#ch1_vitality 等），空表示固定第 0 帧。
	ConditionState string
	// OpacityProperty 是透明度属性名（#briefing.ch1opacity1），空表示不透明。
	OpacityProperty string
	// Opacity 是数值透明度（0..255）；-1 表示没有数值写法。
	Opacity int
	// ValidCondition 为空或求值为真时才画。
	ValidCondition string
	// AttachImage / AttachCondition 是挂在图层上的小动画（work_blink）。
	// 当前只记录，不播放：闪烁是纯装饰，不影响交互和数值。
	AttachImage     string
	AttachCondition string
}

// CustomUIGroup 是一组单选按钮。
type CustomUIGroup struct {
	Type    string
	Result  string
	Buttons []CustomUIButton
}

// CustomUIButton 是单选组里的一个选项。
type CustomUIButton struct {
	Image string
	X, Y  int
	Value int
	// MaxState 是按钮图帧数；定义里通常不写，工作按钮固定 4 帧。
	MaxState int
	// DisableCondition 求值为真时这一项灰掉、不可点。
	DisableCondition string
}

// ParseCustomUIDefinition 解析一份 type=176 定义。
func ParseCustomUIDefinition(data []byte) (*CustomUIDefinition, error) {
	root, err := ParseLayoutDocument(data)
	if err != nil {
		return nil, err
	}

	definition := &CustomUIDefinition{}
	if objects, ok := Array(root["objects"]); ok {
		for _, item := range objects {
			if object, ok := parseCustomUIObject(item); ok {
				definition.Objects = append(definition.Objects, object)
			}
		}
	}
	if groups, ok := Array(root["buttons"]); ok {
		for _, item := range groups {
			if group, ok := parseCustomUIGroup(item); ok {
				definition.Groups = append(definition.Groups, group)
			}
		}
	}
	if conditions, ok := Object(root["conditions"]); ok {
		definition.DisableCondition, _ = Text(conditions["disable"])
	}
	definition.ReturnValue, _ = Number(root["return_value"])
	return definition, nil
}

func parseCustomUIObject(value any) (CustomUIObject, bool) {
	entry, ok := Object(value)
	if !ok {
		return CustomUIObject{}, false
	}
	image, ok := Object(entry["image"])
	if !ok {
		return CustomUIObject{}, false
	}
	name, _ := Text(image["name"])
	if len(name) == 0 {
		return CustomUIObject{}, false
	}
	object := CustomUIObject{Image: name, Opacity: -1}
	if pos, ok := Array(image["pos"]); ok {
		object.X, object.Y = buexPoint(pos)
	}
	object.MaxState, _ = Number(image["max_state"])
	object.ConditionState, _ = Text(image["condition_state"])
	if opacity, ok := Text(image["opacity"]); ok {
		object.OpacityProperty = opacity
	} else if number, ok := Number(entry["opacity"]); ok {
		object.Opacity = number
	} else if number, ok := Number(image["opacity"]); ok {
		object.Opacity = number
	}
	object.ValidCondition, _ = Text(entry["valid_conditions"])
	if attach, ok := Object(entry["attach"]); ok {
		object.AttachImage, _ = Text(attach["name"])
		object.AttachCondition, _ = Text(attach["condition"])
	}
	return object, true
}

func parseCustomUIGroup(value any) (CustomUIGroup, bool) {
	entry, ok := Object(value)
	if !ok {
		return CustomUIGroup{}, false
	}
	group := CustomUIGroup{}
	group.Type, _ = Text(entry["type"])
	group.Result, _ = Text(entry["result"])
	objects, ok := Array(entry["objects"])
	if !ok {
		return group, len(group.Result) > 0
	}
	for _, item := range objects {
		buttonEntry, ok := Object(item)
		if !ok {
			continue
		}
		image, ok := Object(buttonEntry["image"])
		if !ok {
			continue
		}
		name, _ := Text(image["name"])
		if len(name) == 0 {
			continue
		}
		button := CustomUIButton{Image: name, MaxState: 4}
		if pos, ok := Array(image["pos"]); ok {
			button.X, button.Y = buexPoint(pos)
		}
		button.Value, _ = Number(buttonEntry["value"])
		if state, ok := Number(buttonEntry["max_state"]); ok && state > 0 {
			button.MaxState = state
		}
		button.DisableCondition, _ = Text(buttonEntry["disable_condition"])
		group.Buttons = append(group.Buttons, button)
	}
	return group, len(group.Result) > 0 || len(group.Buttons) > 0
}
