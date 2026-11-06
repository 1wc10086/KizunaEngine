package gothic

import (
	"strings"

	"kizuna/engine/core"
)

type GothicMenu struct {
	Id                string
	StatementIndex    int32
	EndStatementIndex int32
	Items             []*GothicMenuItem
}

type GothicMenuItem struct {
	Slot           int32
	Text           string
	StatementIndex int32
}

func NewGothicMenu(id string, statementIndex, endStatementIndex int32, items []*GothicMenuItem) *GothicMenu {
	return &GothicMenu{
		Id:                id,
		StatementIndex:    statementIndex,
		EndStatementIndex: endStatementIndex,
		Items:             items,
	}
}

func NewGothicMenuItem(slot int32, text string, statementIndex int32) *GothicMenuItem {
	return &GothicMenuItem{
		Slot:           slot,
		Text:           text,
		StatementIndex: statementIndex,
	}
}

type gothicMenuDraft struct {
	id                string
	statementIndex    int32
	endStatementIndex int32
	items             []*GothicMenuItem
}

func buildGothicMenuTable(statements []*GothicStatement) []*GothicMenu {
	var drafts []*gothicMenuDraft
	var open []*gothicMenuDraft
	for _, statement := range statements {
		if statement.Kind != GothicStatementCommand {
			continue
		}
		if strings.EqualFold(statement.Name, "menu") {
			if len(statement.Args) == 0 {
				core.Logf("gothic: menu without id ignored.")
				continue
			}
			draft := &gothicMenuDraft{
				id:                statement.Args[0],
				statementIndex:    statement.Index,
				endStatementIndex: -1,
			}
			drafts = append(drafts, draft)
			open = append(open, draft)
		} else if strings.EqualFold(statement.Name, "item") {
			if len(open) == 0 {
				core.Logf("gothic: item outside menu ignored.")
				continue
			}
			top := open[len(open)-1]
			top.items = append(top.items, NewGothicMenuItem(int32(len(top.items)+1), unquoteMenuText(statement.Args), statement.Index+1))
		} else if strings.EqualFold(statement.Name, "endmenu") {
			if len(open) == 0 {
				core.Logf("gothic: endmenu without menu ignored.")
				continue
			}
			top := open[len(open)-1]
			open = open[:len(open)-1]
			top.endStatementIndex = statement.Index
		}
	}
	for range open {
		core.Logf("gothic: menu without endmenu left open.")
	}
	menus := make([]*GothicMenu, 0, len(drafts))
	for _, draft := range drafts {
		menus = append(menus, NewGothicMenu(draft.id, draft.statementIndex, draft.endStatementIndex, draft.items))
	}
	return menus
}

func unquoteMenuText(args []string) string {
	text := strings.TrimSpace(strings.Join(args, " "))
	if len(text) >= 2 && text[0] == '"' && text[len(text)-1] == '"' {
		return text[1 : len(text)-1]
	}
	return text
}
