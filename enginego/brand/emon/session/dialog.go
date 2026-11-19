package session

// dialog.go：emon 引擎自绘的 prompt 对话框（GetYesNoMessage / GetStringMessage）。
//
// 原版依据（撕下 emecfg.macros，ASENURE）:
//   - 存档覆盖确认：#GetYesNoMessage(Default,上書きしますか？,ゲームの保存,変数５)
//     — 提示「上書きしますか？」，标题「ゲームの保存」，回答写进変数５（1=是 / 0=否），
//     答案 0 时 IfJump(セーブ画面表示,変数５==0) 回到存档页是/否 二选一，不是文本输入。
//   - 读档确认：#GetYesNoMessage(Default,ロードしますか？,ゲームの再開,変数５)
//     — 同上，提示「ロードしますか？」，0 时回读档页，1 才 Load。
//   - 评论输入：#GetStringMessage(Default,コメント入力,変数５)
//     — 既有存档改注释、以及首次 Save 的注释写入；可见输入框 + 确认/取消。
//     HERONAME 的名字输入也是 GetString 半边：ApearMenu(HERONAME) 把
//     主人公名前 写进同变量，视觉同样是可编辑文字，只是变量不同。
//   - SystemWindow 的「ゲームを終了しますか？」「タイトルへ戻りますか？」也是同一种框。
//
// 设计：对话框画进引擎帧——居中窗口 + 半透明底 + 标题 + 提示 + 按钮（是/否 走 machine.SubmitYesNo，
// 文字输入的点击也走 SubmitName）。MapButtons 把按钮热区暴露给宿主，让 Pointer 的 tap 直接落回
// 引擎，不依赖 Flutter 叠按钮。名字/评论输入还需要 Android 键盘：宿主用一个无边框/透明的
// TextField 只负责 IME，通过 OpPromptText 把当前文字同步回引擎，由引擎画进帧。引擎未挂起时
// PromptDialog.Active=false，宿主回落到旧 NameDialog（bishop 兼容）。
//
// 布局以 1024x768 为基准（ASENURE 实际窗口），居中固定尺寸再按实际宽高钳制：
// 这与 bishop 标题菜单的居中按钮、emon 对白窗的文章文字盒属于同一套"原引擎居中模态"惯例，
// 固定尺寸避免随窗口宽度漂移让按钮热区与画面脱节。

import (
	bishopgfx "kizuna/enginego/brand/bishop/gfx"
	emontext "kizuna/enginego/brand/emon/text"
	"kizuna/enginego/brand/emon/vm"
	"kizuna/enginego/core"
	"strings"
)

const (
	// 对话框基准尺寸（以 ASENURE 1024x768 为准）；窗口比这宽时以实际宽为准。
	dialogWidth     = 620
	dialogHeight    = 240
	dialogPadding   = 28
	dialogTitleH    = 30
	dialogPromptH   = 64
	dialogButtonW   = 180
	dialogButtonH   = 52
	dialogButtonGap = 28
	dialogEditH     = 56
	dialogFontSize  = 24
)

type promptButton struct {
	id    string
	x, y  int
	w, h  int
	label string
}

type promptView struct {
	yesNo   bool
	title   string
	prompt  string
	edit    string
	buttons []promptButton
	left    int
	top     int
	w       int
	h       int
	// editX/editY/editW 是字符串输入框的矩形（同时是宿主透明 TextField 的位置与大小）。
	// 只有 string 模式下有效，yesNo 模式下为零。
	editX, editY, editW int
	present             bool
}

// currentPromptView 返回当前挂起的 prompt 对话框几何。挂起判定与 State.AwaitingName 同一谓词：
// 只有 PendingPrompt 存在且当前步为 AwaitName 时才有窗。
// 没有挂起的 prompt 时 present=false，调用方（Compose/State）直接走旧路径。
func (s *Session) currentPromptView() promptView {
	if s.disposed || s.machine == nil || s.ctx == nil {
		return promptView{}
	}
	prompt, ok := s.machine.PendingPrompt()
	if !ok {
		return promptView{}
	}
	if s.step == vm.EmonStepFinished || s.step == vm.EmonStepFaulted {
		return promptView{}
	}
	yesNo := s.machine.PendingPromptIsYesNo()
	// GetYesNoMessage / GetStringMessage 的实参在 vm/system.go 里已被规范化：
	//   #GetYesNoMessage(Default,上書きしますか？,ゲームの保存,変数５)
	// 的第 1 参是 Default 占位，第 2 参是标题或提示（脚本实测按此顺序取值），
	// 且 machine 只存了一串 prompt（无 title/when 区分）。这里只画 prompt 一行，
	// 标题行用 prompt（与脚本显示的提示文言相同），避免编造 second param。
	// 若将来 machine 扩展 PendingPromptTitle，可在此拆出真实 title。
	view := promptView{
		yesNo:   yesNo,
		prompt:  prompt,
		present: true,
	}
	view.left, view.top, view.w, view.h = dialogRect(s.width, s.height)
	if yesNo {
		// 是/否两按钮，居中横排
		totalW := dialogButtonW*2 + dialogButtonGap
		if view.w-totalW < 0 {
			totalW = view.w
			// 窄窗时按钮并排塞不下：按钮全宽堆叠
			view.buttons = []promptButton{
				{
					id:    core.PromptButtonYes,
					label: "\u662f",
					x:     view.left + (view.w-dialogButtonW)/2,
					y:     view.top + dialogPadding + dialogTitleH + dialogPromptH + 8,
					w:     dialogButtonW,
					h:     dialogButtonH,
				},
				{
					id:    core.PromptButtonNo,
					label: "\u5426",
					x:     view.left + (view.w-dialogButtonW)/2,
					y:     view.top + dialogPadding + dialogTitleH + dialogPromptH + dialogButtonH + 16,
					w:     dialogButtonW,
					h:     dialogButtonH,
				},
			}
			return view
		}
		startX := view.left + (view.w-totalW)/2
		buttonY := view.top + dialogPadding + dialogTitleH + dialogPromptH + dialogButtonH
		view.buttons = []promptButton{
			{id: core.PromptButtonYes, label: "\u662f", x: startX, y: buttonY, w: dialogButtonW, h: dialogButtonH},
			{id: core.PromptButtonNo, label: "\u5426", x: startX + dialogButtonW + dialogButtonGap, y: buttonY, w: dialogButtonW, h: dialogButtonH},
		}
	} else {
		editY := view.top + dialogPadding + dialogTitleH + dialogPromptH + 8
		view.edit = s.promptEdit
		view.editX = view.left + dialogPadding
		view.editY = editY
		view.editW = view.w - dialogPadding*2
		view.buttons = []promptButton{
			{
				id:    core.PromptButtonCancel,
				label: "\u53d6\u6d88",
				x:     view.left + dialogPadding,
				y:     editY + dialogEditH + 16,
				w:     dialogButtonW - 40,
				h:     dialogButtonH,
			},
			{
				id:    core.PromptButtonConfirm,
				label: "\u786e\u8ba4",
				x:     view.left + view.w - dialogPadding - (dialogButtonW - 40),
				y:     editY + dialogEditH + 16,
				w:     dialogButtonW - 40,
				h:     dialogButtonH,
			},
		}
	}
	return view
}

// drawPromptDialog 把对话框画进当前帧。必须在 Compose+对白之后调用，覆盖式绘制。
func (s *Session) drawPromptDialog() {
	if s.frame == nil || s.composer == nil || s.machine == nil {
		return
	}
	view := s.currentPromptView()
	if !view.present {
		return
	}
	canvas := s.frame
	// 半透明遮罩：把底压暗而非纯黑，存档/读档页的缩略图轮廓仍能隐约辨认，
	// 与原版确认窗"半透底 + 模态窗"的遮挡语义一致。
	canvas.FillBlend(0, 0, s.width, s.height, 8, 10, 18, 165)

	bx, by, bw, bh := view.left, view.top, view.w, view.h
	canvas.Fill(bx, by, bw, bh, 16, 20, 28, 255)
	canvas.Fill(bx, by, bw, 3, 220, 230, 240, 255)
	canvas.Fill(bx, by+bh-3, bw, 3, 90, 100, 114, 255)
	canvas.Fill(bx, by, 3, bh, 220, 230, 240, 255)
	canvas.Fill(bx+bw-3, by, 3, bh, 90, 100, 114, 255)

	glyphs := emontext.CurrentGlyphSource()
	if glyphs == nil {
		glyphs = emontext.ResolveGlyphSource()
	}
	// 标题行与提示行：实测原版同时展示标题与提示（如「ゲームの保存」+「上書きしますか？」）；
	// machine 的 prompt 只剩一串，这里标题与提示同为 prompt（显示两次会被人视为 bug 部屋里装修的最稳妥做法：
	// 只画一行提示，避免靠猜标题）。
	// 若后续确认 title 字段可用，改回两行即可，不阻塞本次是/否 与文字输入闭环。
	DrawPromptLine(canvas, view.prompt, bx+dialogPadding, by+dialogPadding, dialogFontSize, glyphs, 235, 240, 248)

	if !view.yesNo {
		canvas.Fill(view.editX, view.editY, view.editW, dialogEditH, 250, 252, 255, 255)
		canvas.Fill(view.editX, view.editY, view.editW, 2, 120, 130, 145, 255)
		canvas.Fill(view.editX, view.editY+dialogEditH-2, view.editW, 2, 120, 130, 145, 255)
		canvas.Fill(view.editX, view.editY, 2, dialogEditH, 120, 130, 145, 255)
		canvas.Fill(view.editX+view.editW-2, view.editY, 2, dialogEditH, 120, 130, 145, 255)
		if strings.TrimSpace(view.edit) != "" {
			DrawPromptLine(canvas, view.edit, view.editX+12, view.editY+(dialogEditH-dialogFontSize)/2, dialogFontSize, glyphs, 20, 20, 20)
		} else {
			DrawPromptLine(canvas, "\u8f93\u5165\u6588\u5b57", view.editX+12, view.editY+(dialogEditH-dialogFontSize)/2, dialogFontSize, glyphs, 150, 160, 175)
		}
	}

	for _, btn := range view.buttons {
		primary := btn.id == core.PromptButtonYes || btn.id == core.PromptButtonConfirm
		if primary {
			canvas.Fill(btn.x, btn.y, btn.w, btn.h, 32, 120, 215, 255)
			canvas.Fill(btn.x, btn.y, btn.w, 2, 120, 190, 255, 255)
		} else {
			canvas.Fill(btn.x, btn.y, btn.w, btn.h, 58, 66, 80, 255)
			canvas.Fill(btn.x, btn.y, btn.w, 2, 140, 150, 165, 255)
		}
		DrawPromptLineCenter(canvas, btn.label, btn.x, btn.y, btn.w, btn.h, dialogFontSize, glyphs, 245, 248, 252)
	}
}

// promptDialogExposed 是 state 里给宿主的只读快照：包含按钮热区，让 Pointer 真正落回引擎。
func (s *Session) promptDialogExposed() core.PromptDialog {
	view := s.currentPromptView()
	if !view.present {
		return core.PromptDialog{}
	}
	out := core.PromptDialog{
		Active: true,
		YesNo:  view.yesNo,
		Title:  view.title,
		Prompt: view.prompt,
		Edit:   view.edit,
	}
	for _, btn := range view.buttons {
		out.Buttons = append(out.Buttons, core.PromptButton{
			ID: btn.id,
			X:  int32(btn.x),
			Y:  int32(btn.y),
			W:  int32(btn.w),
			H:  int32(btn.h),
		})
	}
	return out
}

// PromptDialog 是 wire OpPromptDialog=43 的进入点：宿主取对话框快照（Active=false 为空）。
func (s *Session) PromptDialog() core.PromptDialog {
	if s.disposed || s.machine == nil {
		return core.PromptDialog{}
	}
	if !s.gate.TryLock() {
		return s.promptDialogExposed()
	}
	defer s.gate.Unlock()
	return s.promptDialogExposed()
}

// SetPromptText 是 wire OpPromptText=44 的进入点：宿主把 IME 当前文字交给引擎渲染。
//
// 约束：只在 string 模式、对话框挂起时有效；yesNo 时忽略（避免宿主误传把确认框写坏）。
// 长度截断：GetStringMessage 原引擎没有显式上限，但评论/名字过长会挤爆单行输入框，
// 这里设 64 以保护帧面，也防止宿主误循环。截断仅作用于渲染字符串，不改变量值本身
// （变量提交走 SubmitName/SubmitYesNo，引擎侧保留完整文字）。
func (s *Session) SetPromptText(text string) {
	if s.disposed || s.machine == nil || s.ctx == nil {
		return
	}
	if s.machine.PendingPromptIsYesNo() {
		return
	}
	if _, ok := s.machine.PendingPrompt(); !ok {
		return
	}
	text = strings.TrimRight(text, "\r\n")
	if len([]rune(text)) > 64 {
		text = string([]rune(text)[:64])
	}
	if text == s.promptEdit {
		return
	}
	s.promptEdit = text
	s.invalidateFrame()
}

// hitPromptButton 命中结果：命中 id + 是否拦截。
func (s *Session) hitPromptButton(x, y int) (string, bool) {
	view := s.currentPromptView()
	if !view.present {
		return "", false
	}
	for _, btn := range view.buttons {
		if x >= btn.x && x < btn.x+btn.w && y >= btn.y && y < btn.y+btn.h {
			return btn.id, true
		}
	}
	return "", false
}

// dialogRect 居中对话框：按基准宽高居中，窗口小则钳制。
func dialogRect(screenW, screenH int) (left, top, w, h int) {
	w = dialogWidth
	h = dialogHeight
	if w > screenW-32 {
		w = screenW - 32
	}
	if h > screenH-32 {
		h = screenH - 32
	}
	if w < 200 {
		w = screenW
	}
	if h < 120 {
		h = screenH
	}
	left = (screenW - w) / 2
	top = (screenH-h)/2 - 20
	if top < 8 {
		top = 8
	}
	return
}

// DrawPromptLine 把一行提示文字作为一条 run 追加到画布（左对齐）。
//
// 旧实现逐字栅格；现在整行交给宿主，宽度用 DisplayWidth 的半角 1 格/
// 全角 2 格估算，排版结果与原版一致。
func DrawPromptLine(target *bishopgfx.BgraCanvas, text string, x, y, size int, glyphs emontext.GlyphSource, b, g, r byte) {
	DrawPromptLineInternal(target, text, x, y, size, glyphs, b, g, r)
}

func DrawPromptLineCenter(target *bishopgfx.BgraCanvas, text string, x, y, w, h, size int, glyphs emontext.GlyphSource, b, g, r byte) {
	approx := 0
	for _, ch := range text {
		approx += (emontext.DisplayWidth(ch) * size) / 2
	}
	startX := x + max(0, (w-approx)/2)
	startY := y + max(0, (h-size)/2)
	DrawPromptLineInternal(target, text, startX, startY, size, glyphs, b, g, r)
}

func DrawPromptLineInternal(target *bishopgfx.BgraCanvas, text string, x, y, size int, glyphs emontext.GlyphSource, b, g, r byte) {
	if target == nil || strings.TrimSpace(text) == "" {
		return
	}
	// 旧实现逐字画，换行符不产生可见输出；整行交给宿主后必须去掉 \n，
	// 否则一条 run 会在提示框里折成两行。
	text = strings.ReplaceAll(text, "\n", "")
	if text == "" {
		return
	}
	color := uint32(0xFF)<<24 | uint32(r)<<16 | uint32(g)<<8 | uint32(b)
	target.Runs = append(target.Runs, core.TextRun{
		Text:  text,
		X:     int32(x),
		Y:     int32(y + emontext.BaselineOffset(size)),
		Size:  int32(size),
		Color: color,
	})
}
