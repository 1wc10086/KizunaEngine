package bdt

type bdtExpressionNode interface {
	evaluate(resolve func(byte, int32) int32) int32
}

type BdtExpression struct {
	root  bdtExpressionNode
	valid bool
}

func newValidBdtExpression(root bdtExpressionNode) BdtExpression {
	return BdtExpression{root: root, valid: root != nil}
}

func invalidBdtExpression() BdtExpression {
	return BdtExpression{}
}

func (e BdtExpression) IsValid() bool {
	return e.valid && e.root != nil
}

func (e BdtExpression) Evaluate(resolve func(byte, int32) int32) (result int32) {
	if !e.IsValid() || resolve == nil {
		return -1
	}
	defer func() {
		if recover() != nil {
			result = -1
		}
	}()
	return e.root.evaluate(resolve)
}

type bdtConstantNode struct {
	value int32
}

func (n *bdtConstantNode) evaluate(resolve func(byte, int32) int32) int32 {
	return n.value
}

type bdtVariableNode struct {
	bank  byte
	index int32
}

func (n *bdtVariableNode) evaluate(resolve func(byte, int32) int32) int32 {
	return resolve(n.bank, n.index)
}

type bdtGroupNode struct {
	items []bdtExpressionNode
}

func (n *bdtGroupNode) evaluate(resolve func(byte, int32) int32) int32 {
	if len(n.items) == 0 {
		return -1
	}
	return n.items[len(n.items)-1].evaluate(resolve)
}

type bdtApplyNode struct {
	operator  byte
	arguments []bdtExpressionNode
}

func (n *bdtApplyNode) evaluate(resolve func(byte, int32) int32) int32 {
	switch n.operator {
	case '&':
		if len(n.arguments) == 0 {
			return -1
		}
		for _, argument := range n.arguments {
			if argument.evaluate(resolve) == 0 {
				return 0
			}
		}
		return 1
	case '|':
		if len(n.arguments) == 0 {
			return -1
		}
		for _, argument := range n.arguments {
			if argument.evaluate(resolve) != 0 {
				return 1
			}
		}
		return 0
	case '=':
		if len(n.arguments) == 0 {
			return -1
		}
		if len(n.arguments) == 1 {
			return n.arguments[0].evaluate(resolve)
		}
		first := n.arguments[0].evaluate(resolve)
		for i := 1; i < len(n.arguments); i++ {
			if n.arguments[i].evaluate(resolve) != first {
				return 0
			}
		}
		return 1
	case '!':
		if len(n.arguments) == 1 {
			if n.arguments[0].evaluate(resolve) == 0 {
				return 1
			}
			return 0
		}
		if len(n.arguments) == 2 {
			if n.arguments[0].evaluate(resolve) != n.arguments[1].evaluate(resolve) {
				return 1
			}
			return 0
		}
		return -1
	case '<':
		if len(n.arguments) != 2 {
			return -1
		}
		if n.arguments[0].evaluate(resolve) < n.arguments[1].evaluate(resolve) {
			return 1
		}
		return 0
	case '>':
		if len(n.arguments) != 2 {
			return -1
		}
		if n.arguments[0].evaluate(resolve) > n.arguments[1].evaluate(resolve) {
			return 1
		}
		return 0
	default:
		return -1
	}
}
