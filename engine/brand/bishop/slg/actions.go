package slg

import (
	"strconv"
	"strings"

	"kizuna/engine/brand/bishop/data/scriptdb"
)

// Host 是 SLG 宿主命令需要的剧本侧能力。
//
// 全部做成窄方法：这些能力分散在 session 的不同实现里（变量表、存档、
// 界面栈、鉴赏率），分派器只依赖"变量 + DB + 跳转"，不反过来依赖 session，
// 这样它可以在测试里用假宿主驱动，也不会把 session 的依赖拖进数据层。
type Host interface {
	// DB 返回数值表；nil 表示本作没有 var_db，$ 取值一律按 0。
	DB() *scriptdb.DB
	// Property/SetProperty 按名字读写剧本变量（#var / @var），未知变量读 0。
	Property(name string) int32
	SetProperty(name string, value int32)
	// PrivateData/CommonData 是 Lua 同名全局表：JSON 可序列化，
	// 分别对应"每存档一份"与"跨周目共享"。
	PrivateData() map[string]any
	CommonData() map[string]any
	UnlockAchievement(ids ...string)
	// EnterState 切换 SLG 场景（training / territory / day_result）。
	EnterState(name string)
	// CustomMap 进入自定义地图画面，返回值写回剧本（0 表示没进入）。
	CustomMap() int32
	// Movie 请求播放影片；false 表示这个名字不是影片。
	Movie(name string) bool
	Title() bool
	// SetGalleryRate 返回鉴赏室某类内容的达成率（0..100），写回剧本。
	SetGalleryRate(kind string) int32
	// WaitAnimating 在变量还没到目标值时返回 0，脚本据此原地重试。
	WaitAnimating(varName string, value int32) int32
	ActionType(name string) int32
	GetObjectTest(varName string) int32
	// CallLabel 跳到剧本标签（NextTraining 用），false 表示标签不存在。
	CallLabel(label string) bool
	// AutoSave 在行动/选择画面出现前写自动存档槽。
	AutoSave()
}

// evaluator 把宿主与 DB 视图打包给语法节点用。
type evaluator struct {
	host Host
	// cached 记住上次建好的视图，避免每条语句都取一遍 DB。
	cached *dbView
}

func (e *evaluator) db() *scriptdb.DB {
	if e.host == nil {
		return nil
	}
	return e.host.DB()
}

func (e *evaluator) view() *dbView {
	if e.cached == nil {
		db := e.db()
		if db == nil {
			return nil
		}
		e.cached = &dbView{db: db}
	}
	return e.cached
}

// dispatcher 是分派器的每次调用状态：宿主 + 求值器。
type dispatcher struct {
	host     Host
	evaluate evaluator
}

func newDispatcher(host Host) *dispatcher {
	return &dispatcher{host: host, evaluate: evaluator{host: host}}
}

// ------------------------------------------------------------------ 语句分派

// Tail 处理 SELECT_TAIL 的 frame[1] 字符串，返回写回 frame[0] 的值。
//
// frame[1] 同时承载两种语义：原地表达式（`#w = ...`、`$#ID_BP[0,'状態'] |= 0x1`，
// 结果必须回写）与宿主命令（`state training`、`実績解除 ID17`）。
// 原引擎先尝试表达式、失败再当命令，这里保持同一种顺序；一条串里可以有
// 多条用分号分开的语句，最后一条表达式的结果作为返回值。
func Tail(host Host, text string) int32 {
	return newDispatcher(host).tails(text)
}

func (d *dispatcher) tails(text string) int32 {
	var result int32
	for _, statement := range splitStatements(text) {
		if value, ok := d.statement(statement); ok {
			result = value
			continue
		}
		if value, ok := d.command(statement); ok {
			result = value
			continue
		}
		// 未识别语句不改变 frame[0]：SELECT_TAIL 之后脚本常用 frame[0]
		// 做分支判断，乱写一个值会让剧情走进错误分支。
	}
	return result
}

// Translate 处理 TRANSLATE 调用，返回值不回写剧本。
func Translate(host Host, text string) {
	newDispatcher(host).translates(text)
}

func (d *dispatcher) translates(text string) {
	for _, statement := range splitStatements(text) {
		if d.translated(statement) {
			continue
		}
		// TRANSLATE 里偶尔混着表达式语句，结果无处可写，执行完丢弃。
		d.statement(statement)
	}
}

// statement 执行一条表达式语句并返回它的值。
// ok=false 表示这条语句不是表达式（交给命令表），而不是求值失败。
func (d *dispatcher) statement(statement string) (int32, bool) {
	if assignment, ok := splitAssignment(statement); ok {
		if value, err := parseExpression(assignment.value); err == nil {
			return d.assign(assignment, value), true
		}
		return 0, false
	}
	if !isPureExpression(statement) {
		return 0, false
	}
	body, err := parseExpression(statement)
	if err != nil {
		return 0, false
	}
	return body.eval(&d.evaluate), true
}

// assignment 是拆开的 `目标 = / += / -= 表达式`。
type assignment struct {
	target string
	op     string
	value  string
	// ref 是 DB 目标（$#表[...]）解析后的形状；普通变量目标留空。
	ref dbRef
}

// splitAssignment 在引号与括号之外找第一个赋值运算符。
//
// 只认 `#name` / `@name` / `$#表[...]` 这三种目标：`=` 在剧本里也出现在
// 比较表达式的"结果赋值"里（`#w = a > b` 是赋值），所以必须以目标形状为准，
// 不能靠"右边能不能解析"反推。
func splitAssignment(statement string) (assignment, bool) {
	var quote byte
	depth := 0
	for index := 0; index < len(statement); index++ {
		character := statement[index]
		switch {
		case quote != 0:
			if character == quote {
				quote = 0
			}
		case character == '\'' || character == '"':
			quote = character
		case character == '[' || character == '(':
			depth++
		case character == ']' || character == ')':
			if depth > 0 {
				depth--
			}
		case depth == 0 && character == '=':
			// == / <= / >= / != 不是赋值。
			if index+1 < len(statement) && statement[index+1] == '=' {
				index++
				continue
			}
			op := "="
			target := ""
			if index > 0 {
				switch statement[index-1] {
				case '+', '-', '|', '&', '*', '/':
					op = statement[index-1 : index+1]
					target = statement[:index-1]
				}
			}
			if op == "=" {
				target = statement[:index]
			}
			target = strings.TrimSpace(target)
			ref, ok := assignable(target)
			if !ok {
				return assignment{}, false
			}
			return assignment{
				target: target,
				op:     op,
				value:  strings.TrimSpace(statement[index+1:]),
				ref:    ref,
			}, true
		}
	}
	return assignment{}, false
}

// assignable 判断目标形状并解析 DB 目标。
func assignable(target string) (dbRef, bool) {
	if strings.HasPrefix(target, "$") {
		lexer := &lexer{text: target}
		value := lexer.next()
		if value.kind != tokenDB || lexer.peek().kind != tokenEOF {
			return dbRef{}, false
		}
		return value.ref, true
	}
	if len(target) > 1 && (target[0] == '#' || target[0] == '@') &&
		!strings.ContainsAny(target, " ()+-*/%<>=!&|") {
		return dbRef{}, true
	}
	return dbRef{}, false
}

func (d *dispatcher) assign(target assignment, value node) int32 {
	result := value.eval(&d.evaluate)
	if target.ref.table != "" {
		view := d.evaluate.view()
		if view == nil {
			return result
		}
		// 位运算写入必须走读-改-写：Add 会把 `|= 0x1` 变成 `+= 1`，
		// 重复执行时状态值会错位（剧本用它置"バイブ/ローター"等标志位）。
		combined := combine(target.op, view.get(target.ref), result)
		view.set(target.ref, combined)
		return combined
	}
	if target.op == "=" {
		d.host.SetProperty(target.target, result)
		return result
	}
	current := d.host.Property(target.target)
	combined := combine(target.op, current, result)
	d.host.SetProperty(target.target, combined)
	return combined
}

// combine 把 `+= n` 这类复合赋值落成一次写入。
func combine(op string, current, value int32) int32 {
	switch op {
	case "+=":
		return current + value
	case "-=":
		return current - value
	case "*=":
		return current * value
	case "/=":
		if value == 0 {
			return 0
		}
		return current / value
	case "|=":
		return current | value
	case "&=":
		return current & value
	}
	return value
}

// isPureExpression 过滤掉"看起来能当表达式解析、其实是命令"的字符串。
// 例如 `title` 单独一个词会被词法器读成未知变量（值 0），如果不拦，
// statement 会把它当成功表达式吞掉，命令永远执行不到。
func isPureExpression(statement string) bool {
	trimmed := strings.TrimSpace(statement)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "$") || trimmed[0] == '#' || trimmed[0] == '@' {
		return true
	}
	if trimmed[0] == '(' || trimmed[0] == '!' || trimmed[0] == '~' {
		return true
	}
	return trimmed[0] >= '0' && trimmed[0] <= '9' || trimmed[0] == '-'
}

// ------------------------------------------------------------------ 命令表

// command 执行一条宿主命令，返回写回 frame[0] 的值；ok=false 表示不认识。
//
// 判定一律"整条语句匹配"或"命令词前缀"，不做逐词模糊匹配。前缀表里
// `state ` 带空格，正好把 `state` 本身排除；`custom map` 是两词整体。
//
// 返回 1 而不是命令自己产生的值：原引擎里 SELECT_TAIL 的 frame[0] 是
// "这条尾命令是否执行成功"，脚本随后写 `NE frame[0]` / `== 0` 判断；
// 真正的结果（达成率、物件编号）由命令写进它指定的变量（#museum_cg_rate、
// #item），两者混用会让 `NE frame[0]` 在结果为 0 时误判失败。
func (d *dispatcher) command(statement string) (int32, bool) {
	switch {
	case statement == "custom map":
		d.writeback(d.host.CustomMap())
		return 1, true
	case statement == "auto save":
		// 原版在行动/选择画面出现前写一次自动存档；脚本在这里已经推进到
		// SELECT_TAIL 之后，读档会从下一条指令继续、重新画出行动面板，
		// 所以可以安全地写 AutoSlot。之前因为"界面中间态不在 SaveData 里"
		// 直接跳过，玩家实际上没有自动存档。
		d.host.AutoSave()
		return 1, true
	case statement == "store flags", statement == "restore flags":
		// 午后命令画面的存档点：先 store 一份基础状态，玩家选完命令后
		// restore。引擎的现代 VM 只有一份属性/变量表，命令画面在选择与
		// restore 之间没有修改游戏状态（只画按钮），所以按"同步完成"
		// 处理即可；关键是必须吞掉这两个 SELECT_TAIL，不能让它们落进
		// 地图选择，否则选择画面会原地重开、命令永远执行不到。
		return 1, true
	case statement == "sync":
		// sync 让引擎等画面同步；引擎没有异步画面，视作立即完成。
		return 1, true
	case statement == "title":
		d.writeback(boolValue(d.host.Title()))
		return 1, true
	case statement == "op" || statement == "staffroll":
		// 影片名由作品决定，这两个是 SLG 固定的片头/片尾。
		d.writeback(boolValue(d.host.Movie(statement)))
		return 1, true
	case strings.HasPrefix(statement, "state "):
		d.host.EnterState(strings.TrimSpace(statement[len("state "):]))
		return 1, true
	case strings.HasPrefix(statement, "実績解除"):
		ids := strings.Fields(statement[len("実績解除"):])
		if len(ids) == 0 {
			// 少了 ID 说明脚本写坏了；返回 0 让无条件重试的循环暴露出来，
			// 而不是静默当成"已解锁"。
			return 0, true
		}
		d.host.UnlockAchievement(ids...)
		return 1, true
	case strings.HasPrefix(statement, "達成率"):
		d.galleryRate(statement[len("達成率"):])
		return 1, true
	case strings.HasPrefix(statement, "is animeted"):
		// 例外：这条命令的返回值本身就是"等待是否结束"。
		// 动画没播完时必须回 0，SELECT_TAIL 才会被脚本重发；
		// 回 1 会让剧情在动画播到一半时继续往下走。
		return d.waitAnimating(statement[len("is animeted"):]), true
	case strings.HasPrefix(statement, "ActionType"):
		d.actionType(strings.TrimSpace(statement[len("ActionType"):]))
		return 1, true
	}
	return 0, false
}

// writeback 把命令结果写进剧本的 `#result` 通道。
// is animeted / NextTraining 之后脚本都用 `NE frame[0]` 或 `#result` 取结果，
// 两个通道同时维护，调用点就不用区分命令类型。
func (d *dispatcher) writeback(value int32) {
	if d.host != nil {
		d.host.SetProperty("#result", value)
	}
}

// galleryRate 解析 `達成率 #museum_cg_rate,cg`：变量名 + 种类。
func (d *dispatcher) galleryRate(argument string) {
	name, kind := splitPair(argument)
	if name == "" {
		return
	}
	d.host.SetProperty(name, d.host.SetGalleryRate(kind))
}

// waitAnimating 解析 `is animeted #result , 1` 并返回等待结果。
//
// 语义是"等到变量等于目标值"：动画没播完时返回 0，脚本会原地重试；
// 播完了返回非 0，SELECT_TAIL 后的 `NE frame[0]` 才会放行。
// 目标值和当前值相同也算完成（0 帧动画）。
func (d *dispatcher) waitAnimating(argument string) int32 {
	name, raw := splitPair(argument)
	if name == "" {
		return 0
	}
	target, ok := parseInteger(raw)
	if !ok {
		target = 0
	}
	value := d.host.WaitAnimating(name, target)
	d.writeback(value)
	return value
}

// actionType 解析 `ActionType1`（数字直接黏在词尾）。
func (d *dispatcher) actionType(argument string) {
	if argument == "" {
		return
	}
	d.writeback(d.host.ActionType(argument))
}

// splitPair 拆 `名字 , 值`。名字可能是 `#var`，值可能是数字或种类名。
func splitPair(argument string) (string, string) {
	argument = strings.TrimSpace(argument)
	index := strings.IndexByte(argument, ',')
	if index < 0 {
		return strings.TrimSpace(argument), ""
	}
	return strings.TrimSpace(argument[:index]), strings.TrimSpace(argument[index+1:])
}

// ------------------------------------------------------------------ TRANSLATE

// translated 处理 TRANSLATE 的调用字符串。
//
// 剧本把这些调用带参数写成一条字符串：`AddCorrectionValue( '早紀' )`、
// `CheckAndUnlockAchievement( 'ID62', 'ID63' )`。按函数名 + 参数列表分派；
// 参数全是单引号字面量，没有嵌套表达式，所以只按逗号切。
func (d *dispatcher) translated(statement string) bool {
	name, arguments, ok := splitCall(statement)
	if !ok {
		return false
	}
	switch name {
	case "ResultTraining":
		d.writeback(d.resultTraining())
		return true
	case "InitBeginTimeParameters":
		d.initBeginTimeParameters()
		return true
	case "InitBeginTimeCounters":
		d.initBeginTimeCounters()
		return true
	case "CancelDayResult":
		// 取消是"回到当天开始"：把快照里的参数写回 DB，候选表再按当前
		// 计数重新过滤——原 Lua 就是这两步。注意不是重新快照：
		// 重新快照会把"已经加过的值"当成当天起点，取消就永远回不去。
		d.restoreBeginTimeParameters()
		d.updateBeginTimeCounters()
		return true
	case "AddCorrectionValue":
		if len(arguments) > 0 {
			d.addCorrectionValue(arguments[0])
		}
		return true
	case "NextTraining":
		d.nextTraining(argument(arguments, 0))
		return true
	case "CheckAndUnlockAchievement":
		d.checkAndUnlockAchievement(arguments)
		return true
	case "GetObjectTest":
		d.writeback(d.getObjectTest(argument(arguments, 0)))
		return true
	}
	return false
}

func argument(arguments []string, index int) string {
	if index < 0 || index >= len(arguments) {
		return ""
	}
	return arguments[index]
}

// splitCall 拆 `Name( arg1, arg2 )`，参数去引号、去空白。
func splitCall(statement string) (string, []string, bool) {
	statement = strings.TrimSpace(statement)
	open := strings.IndexByte(statement, '(')
	if open <= 0 || !strings.HasSuffix(statement, ")") {
		return "", nil, false
	}
	name := strings.TrimSpace(statement[:open])
	if !isIdentifier(name) {
		return "", nil, false
	}
	body := strings.TrimSpace(statement[open+1 : len(statement)-1])
	if body == "" {
		return name, nil, true
	}
	var arguments []string
	for _, part := range strings.Split(body, ",") {
		part = strings.TrimSpace(part)
		part = strings.Trim(part, "'\"")
		arguments = append(arguments, strings.TrimSpace(part))
	}
	return name, arguments, true
}

func isIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for index := 0; index < len(name); index++ {
		character := name[index]
		switch {
		case character >= 'A' && character <= 'Z', character >= 'a' && character <= 'z', character == '_':
		case index > 0 && character >= '0' && character <= '9':
		default:
			return false
		}
	}
	return true
}

// ------------------------------------------------------------------ 工具

// Number 把剧本里的字面量读成整数；读不出来时返回 0。
// 供宿主实现解析 frame 参数（例如训练目标编号）。
func Number(text string) int32 {
	value, _ := parseInteger(text)
	return value
}

// FormatInt 是 Number 的反向操作，宿主写日志时用。
func FormatInt(value int32) string { return strconv.Itoa(int(value)) }

// IsCommand 判断 SELECT/SELECT_TAIL 的参数是不是宿主命令。
//
// 只在"整条语句匹配命令语法"时返回 true，绝不能把地图标签/影片名当成
// 命令：SELECT_TAIL 同时承担地图与影片请求，误判会让普通 AVG 的地图
// 选择直接消失。因此这里不把"纯数字开头"当成表达式（图片名/编号会命中）。
func IsCommand(text string) bool {
	statement := strings.TrimSpace(text)
	if statement == "" {
		return false
	}
	if _, ok := splitAssignment(statement); ok {
		return true
	}
	switch {
	case statement == "custom map", statement == "sync", statement == "title",
		statement == "op", statement == "staffroll", statement == "auto save",
		statement == "store flags", statement == "restore flags":
		return true
	case strings.HasPrefix(statement, "state "),
		strings.HasPrefix(statement, "実績解除"),
		strings.HasPrefix(statement, "達成率"),
		strings.HasPrefix(statement, "is animeted"),
		strings.HasPrefix(statement, "ActionType"):
		return true
	}
	return IsTranslate(statement)
}

// IsTranslate 判断 TRANSLATE 的参数是否是已知的 Lua 桥函数。
func IsTranslate(text string) bool {
	name, _, ok := splitCall(text)
	if !ok {
		return false
	}
	switch name {
	case "ResultTraining", "InitBeginTimeParameters", "InitBeginTimeCounters",
		"CancelDayResult", "AddCorrectionValue", "NextTraining",
		"CheckAndUnlockAchievement", "GetObjectTest":
		return true
	}
	return false
}
