package bdt

type BdtHost interface {
	SetBackground(name string, effectCode int)
	SetSprite(slot int, name string, x, y, mode int)
	ClearSprite(slot int)
	ClearSprites()
	SetWindow(part int, visible bool)
	ShowMessage(speaker, text, voiceCode string)
	ShowFace(name string)
	PlayVoice(code string)
	SetBgm(track, mode int)
	StopBgm()
	PlaySe(name string)
	SetEffect(effectCode int)
	SetSceneId(sceneId string)
	SetScreenMode(mode int)
	Redraw()
}

type ManinHost interface {
	SetCaption(text string)
	StartManinAttack(target int, timeLimit int)
	StartManinOrganizer()
	StartManinStaffRoll()
}
