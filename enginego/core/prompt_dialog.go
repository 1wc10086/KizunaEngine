package core

// PromptDialogExpose 暴露"引擎自绘的 prompt 对话框"给宿主。
//
// Emon 的 GetYesNoMessage/GetStringMessage 在原引擎里是引擎自己的模态窗
// （ASENURE emecfg 宏に「上書きしますか？」「ロードしますか？」「コメント入力」が
// Script内に直接書かれている ではなく DLL が出すモーダル）——原版 PC 経験では
// GetYesNoMessage 是/否 两按钮、GetStringMessage 是带输入框的光标窗。
// 把对话框画进引擎帧可让对比度/文案/布局与原版一致，宿主只负责键盘与点击。
// 只有 emon session 会实现这个接口：旧品牌（bishop 等）不实现，宿主走原 NameDialog 路径，
// 因此行为不被影响。MapButtons 只在有确认窗时返回按钮；没挂话框时返回 nil，
// 此时宿主仍按旧逻辑显示 NameDialog（例如切到没有文字配置的作品）。
type PromptDialog struct {
	// Active 是否当前有挂起的 prompt（GetYesNoMessage / GetStringMessage 二选一）。
	Active bool
	// YesNo 是询问（是/否）还是文字输入；与 PendingPromptIsYesNo 对应。
	YesNo bool
	// Title 是原脚本给出的标题（"ゲームの保存" 等）；空表示不需要标题行。
	Title string
	// Prompt 是提示文案（"上書きしますか？" 等），由脚本原文传来。
	Prompt string
	// Edit 已输入但尚未提交的文字（通过 OpPromptText 同步，运行时由 IME 写入）。
	// confirm 后为空表示沿用引擎的缺省（投稿文本为空时用主人公名等兜底）。
	Edit string
	// MaxLength 是可输入文字上限：GetStringMessage в EDIT 上限（无上限时为 0）。
	// GetStringMessage 有 6 步数限制（我们用 Encourage）；一旦输入超长，超出部分截断。
	MaxLength int
	// Buttons 是可点按钮（确认/取消等）；坐标是引擎游戏屏幕坐标。
	Buttons []PromptButton
}

type PromptButton struct {
	// ID 是按钮逻辑标识：yes/no/confirm/cancel，由宿主直接提交。
	ID string
	X  int32
	Y  int32
	W  int32
	H  int32
}

// PromptButtonID 是按钮确认对应的语义，Dart 与 Go 只需对应一致。
const (
	PromptButtonYes     = "yes"
	PromptButtonNo      = "no"
	PromptButtonConfirm = "confirm"
	PromptButtonCancel  = "cancel"
)

// PromptDialogExposer 由 emon 实现，返回当前挂起的对话框视图（空表示无挂话框)\).
type PromptDialogExposer interface {
	PromptDialog() PromptDialog
}

// PromptTextSetter 由 emon 实现：宿主把 IME 当前编辑文字交给引擎渲染。
// 引擎在下一帧把该文字画进输入框，避免 Flutter 叠自己的预览文字。
type PromptTextSetter interface {
	SetPromptText(text string)
}
