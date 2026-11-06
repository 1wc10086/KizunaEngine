package bdt

type bdtInfixNode interface {
	evaluate(resolve func(byte, int32) int32) int32
}

type BdtInfixExpression struct {
	root  bdtInfixNode
	valid bool
}

func newValidBdtInfixExpression(root bdtInfixNode) BdtInfixExpression {
	return BdtInfixExpression{root: root, valid: root != nil}
}

func invalidBdtInfixExpression() BdtInfixExpression {
	return BdtInfixExpression{}
}

func (e BdtInfixExpression) IsValid() bool {
	return e.valid && e.root != nil
}

func (e BdtInfixExpression) Evaluate(resolve func(byte, int32) int32) int32 {
	if !e.IsValid() || resolve == nil {
		return 0
	}
	return e.root.evaluate(resolve)
}

type bdtInfixConstantNode struct {
	value int32
}

func (n *bdtInfixConstantNode) evaluate(resolve func(byte, int32) int32) int32 {
	return n.value
}

type bdtInfixVariableNode struct {
	bank  byte
	index int32
}

func (n *bdtInfixVariableNode) evaluate(resolve func(byte, int32) int32) int32 {
	return resolve(n.bank, n.index)
}

type bdtInfixComparisonNode struct {
	operator    byte
	left, right bdtInfixNode
}

func (n *bdtInfixComparisonNode) evaluate(resolve func(byte, int32) int32) int32 {
	left := n.left.evaluate(resolve)
	right := n.right.evaluate(resolve)
	switch n.operator {
	case '=':
		if left == right {
			return 1
		}
		return 0
	case '<':
		if left < right {
			return 1
		}
		return 0
	case '>':
		if left > right {
			return 1
		}
		return 0
	case '!':
		if left != right {
			return 1
		}
		return 0
	default:
		return 0
	}
}

type bdtInfixLogicNode struct {
	operator    byte
	left, right bdtInfixNode
}

func (n *bdtInfixLogicNode) evaluate(resolve func(byte, int32) int32) int32 {
	left := n.left.evaluate(resolve) != 0
	right := n.right.evaluate(resolve) != 0
	switch n.operator {
	case '&':
		if left && right {
			return 1
		}
		return 0
	case '|':
		if left || right {
			return 1
		}
		return 0
	default:
		return 0
	}
}
