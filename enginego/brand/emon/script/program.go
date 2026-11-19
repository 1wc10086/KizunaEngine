package script

import "strings"

type EmonProgram struct {
	Name       string
	Statements []*EmonStatement
	labels     map[string]int32
	labelOrder []string
}

func NewEmonProgram(name string, statements []*EmonStatement) *EmonProgram {
	program := &EmonProgram{
		Name:       name,
		Statements: statements,
		labels:     make(map[string]int32),
	}
	for i := 0; i < len(statements); i++ {
		statement := statements[i]
		if statement == nil || statement.IsText {
			continue
		}

		if !strings.EqualFold(statement.Command, "SceneName") {
			continue
		}

		if statement.ArgumentCount() == 0 {
			continue
		}

		label := statement.Argument(0).Raw
		if _, exists := program.labelIndex(label); !exists {
			program.labels[label] = int32(i)
			program.labelOrder = append(program.labelOrder, label)
		}
	}

	return program
}

func (p *EmonProgram) FindLabel(label string) int32 {
	if index, exists := p.labelIndex(label); exists {
		return index
	}

	return -1
}

func (p *EmonProgram) labelIndex(label string) (int32, bool) {
	if index, exists := p.labels[label]; exists {
		return index, true
	}

	for _, key := range p.labelOrder {
		if strings.EqualFold(key, label) {
			return p.labels[key], true
		}
	}

	return 0, false
}
