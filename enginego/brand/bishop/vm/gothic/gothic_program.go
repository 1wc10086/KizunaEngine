package gothic

import (
	"fmt"
	"os"
	"strings"
)

type GothicProgram struct {
	statements []*GothicStatement
	menus      []*GothicMenu
	variables  []string
	labels     map[string]int32
	menuById   map[string]*GothicMenu
}

func NewGothicProgram(statements []*GothicStatement) *GothicProgram {
	labels := make(map[string]int32)
	for _, statement := range statements {
		if statement.Kind == GothicStatementLabel {
			lower := strings.ToLower(statement.Name)
			if _, ok := labels[lower]; !ok {
				labels[lower] = statement.Index
			}
		}
	}
	menus := buildGothicMenuTable(statements)
	menuById := make(map[string]*GothicMenu)
	for _, menu := range menus {
		lower := strings.ToLower(menu.Id)
		if _, ok := menuById[lower]; !ok {
			menuById[lower] = menu
		}
	}
	var variables []string
	for _, statement := range statements {
		if statement.Kind != GothicStatementCommand || !strings.EqualFold(statement.Name, "int") || len(statement.Args) == 0 {
			continue
		}
		known := false
		for _, declared := range variables {
			if strings.EqualFold(declared, statement.Args[0]) {
				known = true
				break
			}
		}
		if !known {
			variables = append(variables, statement.Args[0])
		}
	}
	return &GothicProgram{
		statements: statements,
		menus:      menus,
		variables:  variables,
		labels:     labels,
		menuById:   menuById,
	}
}

func (p *GothicProgram) Statements() []*GothicStatement {
	return p.statements
}

func (p *GothicProgram) Menus() []*GothicMenu {
	return p.menus
}

func (p *GothicProgram) DeclaredVariables() []string {
	return p.variables
}

func (p *GothicProgram) Count() int {
	return len(p.statements)
}

// FindScenePreviewImage 找出一个回想标签里最有代表性的事件图。
//
// H1SCENE..H21SCENE 本身只负责铺一张背景再 gosub 到具体场景；
// 鉴赏网格要显示的是事件图，而不是所有场景共用的 gb01。这里优先
// 在标签范围内找 gi/end/gc 图，找不到就沿着第一个 gosub 再看一层。
// 返回的是不含扩展名的 stem，调用方自己拼 .gsa。
func (p *GothicProgram) FindScenePreviewImage(label string) string {
	index := p.FindLabel(label)
	if index < 0 {
		return ""
	}
	if name := p.sceneImageInRange(index, 2); name != "" {
		return name
	}
	return ""
}

func (p *GothicProgram) sceneImageInRange(start int32, depth int) string {
	if depth < 0 || start < 0 || int(start) >= len(p.statements) {
		return ""
	}
	fallback := ""
	var gosub string
	for i := int(start) + 1; i < len(p.statements); i++ {
		statement := p.statements[i]
		if statement == nil {
			continue
		}
		if statement.Kind == GothicStatementLabel {
			break
		}
		if statement.Kind != GothicStatementCommand {
			continue
		}
		name := strings.ToLower(statement.Name)
		sub := strings.ToLower(statement.Sub)
		if (name == "g" || name == "graphic") && (sub == "draw" || sub == "layer") {
			for _, arg := range statement.Args {
				image := strings.ToLower(unquoteGothic(arg))
				image = strings.TrimSuffix(image, ".gsa")
				if image == "" {
					continue
				}
				if strings.HasPrefix(image, "gi") || strings.HasPrefix(image, "end") || strings.HasPrefix(image, "gc") {
					return image
				}
				if fallback == "" && (strings.HasPrefix(image, "gb") || strings.HasPrefix(image, "gc")) {
					fallback = image
				}
			}
		}
		if (name == "gosub" || name == "call") && len(statement.Args) > 0 && gosub == "" {
			gosub = unquoteGothic(statement.Args[0])
		}
	}
	if gosub != "" && depth > 0 {
		if target := p.FindLabel(gosub); target >= 0 {
			if name := p.sceneImageInRange(target, depth-1); name != "" {
				return name
			}
		}
	}
	return fallback
}

func LoadGothicProgram(path string) (*GothicProgram, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, err
		}
		return nil, fmt.Errorf("gothic: failed to load program: %w", err)
	}
	reader := NewScsRecordReader(data)
	var statements []*GothicStatement
	var index int32
	for {
		line, offset, ok, err := reader.TryReadLine()
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
		statements = append(statements, GothicParseStatement(line, index, offset))
		index++
	}
	return NewGothicProgram(statements), nil
}

func (p *GothicProgram) FindLabel(name string) int32 {
	if index, ok := p.labels[strings.ToLower(name)]; ok {
		return index
	}
	return -1
}

func (p *GothicProgram) FindMenu(id string) *GothicMenu {
	if menu, ok := p.menuById[strings.ToLower(id)]; ok {
		return menu
	}
	return nil
}
