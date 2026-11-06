package session

import (
	"path/filepath"

	"kizuna/engine/core"
)

// CurrentMovie 返回待播影片的绝对路径，供宿主用 fvp 解码。
//
// 只返回"还没被取走"的那一条：宿主取过一次后引擎把状态清掉，
// 避免每帧都重复触发同一个影片。清理动作放在 ConsumeMovie 里而不是这里，
// 是因为 CurrentMovie 会被宿主在轮询中反复调用，带副作用的读法容易出 bug。
func (s *Session) CurrentMovie() string {
	if s.gameState == nil {
		return ""
	}
	name, ok := s.gameState.Movie()
	if !ok || len(name) == 0 {
		return ""
	}
	// SetMovie 的入参有两种形态：剧本给的是影片"名字"（op / staffroll），
	// 也可能是已经可以直接播放的文件路径。路径本身就是宿主需要的最终结果，
	// 不必再经 MoviePath 按名字反查（反查只认文件名）。
	//
	// 相对路径同样在这里收口成绝对路径：Dart 宿主的 File(path) 以 android
	// 进程的工作目录为基准，和引擎/脚本的写法不一定一致，直接传相对路径
	// 会打不开影片。绝对路径原样返回，避免多做一次无意义的系统调用。
	if looksLikeMovieFilePath(name) {
		if filepath.IsAbs(name) {
			return name
		}
		if absolute, err := filepath.Abs(name); err == nil {
			return absolute
		}
		return name
	}
	path, found := s.gameState.MoviePath(name)
	if !found {
		return ""
	}
	return path
}

// looksLikeMovieFilePath 判断 SetMovie 收到的是不是文件路径而不是影片名字。
//
// 判据是"含目录分隔符且带扩展名"：剧本里的影片名（op、staffroll）两者都没有，
// 不会误判；相对路径（movie/op.mpg）会命中，由调用方再补成绝对路径。
func looksLikeMovieFilePath(value string) bool {
	if lastPathSeparator(value) < 0 {
		return false
	}
	// 从尾部往前找扩展名分隔符，遇到路径分隔符说明这是目录名而不是扩展名。
	for i := len(value) - 1; i >= 0; i-- {
		switch value[i] {
		case '.':
			return i > 0 && i < len(value)-1
		case '/', '\\':
			return false
		}
	}
	return false
}

// MovieSerial 返回影片请求序号。
func (s *Session) MovieSerial() int {
	if s.gameState == nil {
		return 0
	}
	return s.gameState.MovieSerial()
}

// ConsumeMovie 在宿主接手播放后清掉影片状态。
//
// 清掉之后脚本才会继续（见 runtime.MovieDirector.Request 的说明：
// 影片期间脚本必须等）。宿主播完调 OpMovieDone 即可推进。
func (s *Session) ConsumeMovie() {
	if s.gameState == nil {
		return
	}
	if _, ok := s.gameState.Movie(); !ok {
		return
	}
	s.gameState.ClearMovie()
	core.Logf("movie: handed to the host player")
}

// FinishMovie 处理"影片播完了"这件事，返回引擎接下来的状态。
//
// 两条来源的后续完全不同，必须由引擎区分：
//   - 标题画面的 OP：回到主界面（titleMode 仍是 true），让玩家继续选菜单；
//   - 剧本里的影片：脚本原本停在 AwaitMovie，继续往下执行。
//
// 交给宿主去猜会让"播完 OP 直接进剧情"这种错误发生。
func (s *Session) FinishMovie() (core.Step, error) {
	// 自己清一次影片状态，而不是依赖调用方先调 ConsumeMovie：
	// "播完"和"清状态"是同一件事的两面，分开做就会出现"忘了清 -> 下一帧
	// 又被当成新影片请求"的循环。这里做成幂等，重复调用没有副作用。
	s.ConsumeMovie()
	if s.titleMode {
		// 标题 OP：什么都不推进，重新回到标题。
		// 不调 enterTitle 重建界面——菜单和底图都还在，重建会白丢一次合成缓存。
		core.Logf("movie: title opening finished, back to the title screen")
		s.lastStep = core.StepAwaitTitle
		return core.StepAwaitTitle, nil
	}
	// 剧本影片：脚本之前停在 AwaitMovie，这里继续执行。
	return s.finishRun(s.run(commandBudget)), nil
}

// requestStaffrollMovie 在没有字幕素材（staffroll_list）的旧作里改用 staffroll 影片。
//
// 旧作的片尾只有 staffroll.avi，没有引擎画的字幕图：此时交给宿主播片，播完后脚本
// 继续执行，和 op 走同一条 AwaitMovie 通道。影片解析不到时返回 false，调用方保持原行为。
func (s *Session) requestStaffrollMovie() bool {
	if s.gameState == nil {
		return false
	}
	const name = "staffroll"
	if _, ok := s.gameState.MoviePath(name); !ok {
		return false
	}
	s.gameState.SetMovie(name)
	s.gameState.SetMovieSerial(s.gameState.MovieSerial() + 1)
	core.Logf("ui: staffroll without staffroll_list -> movie '%s'", name)
	return true
}

// lastPathSeparator 返回最后一个路径分隔符的下标；没有分隔符时返回 -1。
func lastPathSeparator(value string) int {
	for i := len(value) - 1; i >= 0; i-- {
		if value[i] == '/' || value[i] == '\\' {
			return i
		}
	}
	return -1
}
