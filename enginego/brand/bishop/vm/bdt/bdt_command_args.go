package bdt

type BdtCommandArgs struct {
	body     string
	position int
}

func ParseBdtCommandArgs(body string) *BdtCommandArgs {
	return &BdtCommandArgs{body: body}
}

func (a *BdtCommandArgs) AtEnd() bool {
	return a.skipSeparators(a.position) >= len(a.body)
}

func (a *BdtCommandArgs) Remainder() string {
	position := a.skipSeparators(a.position)
	if position >= len(a.body) {
		return ""
	}
	return a.body[position:]
}

func (a *BdtCommandArgs) Next() string {
	start := a.skipSeparators(a.position)
	if start >= len(a.body) {
		a.position = start
		return ""
	}
	end := start
	for end < len(a.body) && a.body[end] != ',' {
		end++
	}
	trim := end
	for trim > start && isBdtBlank(a.body[trim-1]) {
		trim--
	}
	if end < len(a.body) {
		a.position = end + 1
	} else {
		a.position = end
	}
	return a.body[start:trim]
}

func (a *BdtCommandArgs) NextInt() int32 {
	return bdtAtoi(a.Next())
}

func (a *BdtCommandArgs) skipSeparators(position int) int {
	for position < len(a.body) && isBdtSeparator(a.body[position]) {
		position++
	}
	return position
}

func isBdtSeparator(value byte) bool {
	return value == ',' || value == ' ' || value == '\t'
}

func isBdtBlank(value byte) bool {
	return value == ' ' || value == '\t'
}

func bdtAtoi(value string) int32 {
	if len(value) == 0 {
		return 0
	}
	i := 0
	for i < len(value) && (value[i] == ' ' || value[i] == '\t') {
		i++
	}
	sign := int64(1)
	if i < len(value) && (value[i] == '+' || value[i] == '-') {
		if value[i] == '-' {
			sign = -1
		}
		i++
	}
	var total int64
	any := false
	for i < len(value) && value[i] >= '0' && value[i] <= '9' {
		any = true
		total = total*10 + int64(value[i]-'0')
		if total > 2000000000 {
			break
		}
		i++
	}
	if !any {
		return 0
	}
	signed := sign * total
	if signed > 2147483647 {
		return 2147483647
	}
	if signed < -2147483648 {
		return -2147483648
	}
	return int32(signed)
}
