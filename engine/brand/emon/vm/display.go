package vm

import (
	"math"
	"strconv"
	"strings"
	"time"

	bishopgfx "kizuna/engine/brand/bishop/gfx"
	emongfx "kizuna/engine/brand/emon/gfx"
	"kizuna/engine/brand/emon/script"
	"kizuna/engine/brand/emon/stage"
	emontext "kizuna/engine/brand/emon/text"
	"kizuna/engine/core"
)

const (
	resourceClearBlack = 0x200
	resourceColor      = 0x10
	resourceMask       = 0x1000

	colorHighlightBlue  = 0xFF
	colorHighlightGreen = 0xFF
	colorHighlightRed   = 0xFF
	colorHighlightAlpha = 128

	motionBaseMask   = 0xFFFF
	motionFromThis   = 0x400000
	motionF3DRollOn  = 13
	motionF3DRollOff = 14
	motionF3DMove    = 15

	skipModeNoControl = 0x4
	skipModeNoClick   = 0x10
	skipModeNoEffect  = 0x20
)

type ActiveMotion struct {
	Motion     *stage.EmonSpriteMotion
	Start      string
	Completion string
	Touched    time.Time
}

func (m *Machine) MotionsActive() bool {
	return len(m.motions) != 0
}

func (m *Machine) messageBoxSprite() string {
	if value, ok := m.spriteParams.Get("#MsgBox"); ok {
		return value
	}
	return ""
}

func (m *Machine) setMessageBoxSprite(value string) {
	m.spriteParams.Set("#MsgBox", value)
}

func (m *Machine) messageHitSprite() string {
	if value, ok := m.spriteParams.Get("#MsgHit"); ok {
		return value
	}
	return ""
}

func (m *Machine) setMessageHitSprite(value string) {
	m.spriteParams.Set("#MsgHit", value)
}

func (m *Machine) messagePageSprite() string {
	if value, ok := m.spriteParams.Get("#MsgPage"); ok {
		return value
	}
	return ""
}

func (m *Machine) setMessagePageSprite(value string) {
	m.spriteParams.Set("#MsgPage", value)
}

func (m *Machine) cursorSprite() string {
	if value, ok := m.spriteParams.Get("#Cursor"); ok {
		return value
	}
	return ""
}

func (m *Machine) setCursorSprite(value string) {
	m.spriteParams.Set("#Cursor", value)
}

func (m *Machine) cursorX() int {
	if text, ok := m.spriteParams.Get("#CursorX"); ok {
		if value, ok := tryParseInt(text); ok {
			return value
		}
	}
	return 0
}

func (m *Machine) setCursorX(value int) {
	m.spriteParams.Set("#CursorX", strconv.Itoa(value))
}

func (m *Machine) cursorY() int {
	if text, ok := m.spriteParams.Get("#CursorY"); ok {
		if value, ok := tryParseInt(text); ok {
			return value
		}
	}
	return 0
}

func (m *Machine) setCursorY(value int) {
	m.spriteParams.Set("#CursorY", strconv.Itoa(value))
}

func (m *Machine) onTextCommand(statement *script.EmonStatement) EmonStep {
	if m.isReplaying() {
		return EmonStepContinue
	}
	if statement.ArgumentCount() != 0 {
		m.context.Message.Append(m.text(statement, 0))
		m.context.Message.SetVisible(true)
		m.applyMessageIndicators(EmonStepContinue)
	}
	return EmonStepContinue
}

func (m *Machine) onPage(statement *script.EmonStatement) EmonStep {
	if m.isReplaying() {
		return EmonStepContinue
	}
	if statement.ArgumentCount() != 0 {
		m.logOnce("page arguments are ignored")
	}
	m.context.Message.RevealAll()
	if m.context.Message.Body() != "" {
		m.AddHistory(m.context.Message.Body())
	}
	if m.skip {
		m.StopVoiceSound()
		m.context.Message.Clear()
		m.applyMessageIndicators(EmonStepContinue)
		return EmonStepContinue
	}
	m.applyMessageIndicators(EmonStepAwaitInput)
	return EmonStepAwaitInput
}

func (m *Machine) onReturn(statement *script.EmonStatement) EmonStep {
	if m.isReplaying() {
		return EmonStepContinue
	}
	if statement.ArgumentCount() != 0 {
		m.logOnce("return arguments are ignored")
	}
	m.context.Message.RevealAll()
	if m.skip {
		m.StopVoiceSound()
		m.context.Message.Clear()
		m.applyMessageIndicators(EmonStepContinue)
		return EmonStepContinue
	}
	m.applyMessageIndicators(EmonStepAwaitInput)
	return EmonStepAwaitInput
}

func (m *Machine) onWait(statement *script.EmonStatement) EmonStep {
	if m.isReplaying() {
		return EmonStepContinue
	}
	frames := m.EvalInt(statement, 0, 0)
	if m.skip {
		m.StopVoiceSound()
		return EmonStepContinue
	}
	if frames <= 0 {
		return EmonStepContinue
	}
	m.waitFrames = frames
	m.applyMessageIndicators(EmonStepWait)
	return EmonStepWait
}

func (m *Machine) onTime(statement *script.EmonStatement) EmonStep {
	if m.isReplaying() {
		return EmonStepContinue
	}
	frames := m.EvalInt(statement, 0, 0)
	if m.skip {
		return EmonStepContinue
	}
	if frames <= 0 {
		return EmonStepContinue
	}
	m.waitFrames = frames
	m.applyMessageIndicators(EmonStepWait)
	return EmonStepWait
}

func (m *Machine) onSkipOn(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() != 0 {
		m.logOnce("skip on arguments are ignored")
	}
	m.skip = true
	return EmonStepContinue
}

func (m *Machine) onSkipOff(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() != 0 {
		m.logOnce("skip off arguments are ignored")
	}
	m.skip = false
	return EmonStepContinue
}

func (m *Machine) onSetSkipMode(statement *script.EmonStatement) EmonStep {
	mode := 0
	for index := int32(0); index < statement.ArgumentCount(); index++ {
		mode |= parseSkipMode(m.raw(statement, index))
	}
	m.skipMode = mode
	return EmonStepContinue
}

func parseSkipMode(token string) int {
	mode := 0
	for _, part := range strings.Split(token, "@") {
		switch strings.ToUpper(strings.TrimSpace(part)) {
		case "NOCTRLSKIP":
			mode |= skipModeNoControl
		case "NOCLICKSKIP":
			mode |= skipModeNoClick
		case "NOEFFECTSKIP":
			mode |= skipModeNoEffect
		}
	}
	return mode
}

func (m *Machine) onAutoOn(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() != 0 {
		m.logOnce("auto on arguments are ignored")
	}
	m.auto = true
	return EmonStepContinue
}

func (m *Machine) onAutoOff(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() != 0 {
		m.logOnce("auto off arguments are ignored")
	}
	m.auto = false
	return EmonStepContinue
}

func (m *Machine) onClearMessage(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() != 0 {
		m.logOnce("clear message arguments are ignored")
	}
	m.context.Message.Clear()
	m.context.Message.SetVisible(false)
	m.applyMessageIndicators(EmonStepContinue)
	return EmonStepContinue
}

func (m *Machine) applyMessageIndicators(step EmonStep) {
	if step == EmonStepWait {
		m.setMessageIndicator(m.messagePageSprite(), true)
		m.setMessageIndicator(m.messageHitSprite(), false)
		return
	}
	waiting := step == EmonStepAwaitInput && m.context.Message.Body() != "" && m.context.Message.FullyRevealed()
	m.setMessageIndicator(m.messageHitSprite(), waiting)
	m.setMessageIndicator(m.messagePageSprite(), false)
}

func (m *Machine) setMessageIndicator(name string, visible bool) {
	if name == "" {
		return
	}
	sprite := m.context.Stage.Find(name)
	if sprite == nil || sprite.Visible() == visible {
		return
	}
	sprite.SetVisible(visible)
}

func (m *Machine) onMessage(statement *script.EmonStatement) EmonStep {
	text := ""
	if statement.ArgumentCount() != 0 {
		text = m.raw(statement, 0)
	}
	core.Logf("emon: message: %s", text)
	return EmonStepContinue
}

func (m *Machine) onSetSprite(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() < 3 {
		core.Logf("emon: set sprite needs a name, a parent and a kind")
		return EmonStepContinue
	}
	name := m.raw(statement, 0)
	if name == "" {
		core.Logf("emon: set sprite name is empty")
		return EmonStepContinue
	}
	kind := stage.EmonSpriteKind(int(emontext.ParseSpriteKind(m.raw(statement, 2))))
	if isMainSprite(name) {
		root := m.context.Stage.Root()
		root.SetX(m.EvalInt(statement, 3, 0))
		root.SetY(m.EvalInt(statement, 4, 0))
		tokens := []string{}
		for tokenIndex := int32(5); tokenIndex < statement.ArgumentCount(); tokenIndex++ {
			tokens = append(tokens, m.raw(statement, tokenIndex))
		}
		rootFlags := stage.EmonSpriteFlags(int(emontext.ParseSpriteDisplayFlags(strings.Join(tokens, "@"))))
		root.SetFlags(root.Flags() | rootFlags)
		return EmonStepContinue
	}
	parentName := m.raw(statement, 1)
	var parent *stage.EmonSprite
	if parentName == "" || isMainSprite(parentName) {
		parent = m.context.Stage.Root()
	} else if strings.EqualFold(name, parentName) {
		parent = m.context.Stage.Root()
	} else {
		parent = m.context.Stage.Find(parentName)
		if parent == nil {
			parent = m.context.Stage.GetOrCreate(parentName, nil)
		}
	}
	sprite := m.context.Stage.GetOrCreateKind(name, parent, kind)
	sprite.SetKind(kind)
	if kind&stage.EmonSpriteKindGroup != 0 {
		position := m.EvalIntegers(statement, 3)
		if len(position) > 0 {
			sprite.SetX(position[0])
		}
		if len(position) > 1 {
			sprite.SetY(position[1])
		}
		return EmonStepContinue
	}
	sprite.SetX(m.EvalInt(statement, 3, 0))
	sprite.SetY(m.EvalInt(statement, 4, 0))
	var displayToken *string
	var resourceToken *string
	rect := []int{}
	var imageArgument *script.EmonArgument
	for i := int32(5); i < statement.ArgumentCount(); i++ {
		arg := statement.Argument(i)
		raw := arg.Raw
		if raw == "" || strings.EqualFold(raw, "NULL") || strings.EqualFold(raw, "Default") {
			continue
		}
		var expanded string
		if arg.WasQuoted {
			expanded = raw
		} else {
			expanded = m.Lookup(raw).AsString()
		}
		if IsDefaultRectToken(strings.TrimSpace(expanded)) || IsMessageSizeRectToken(strings.TrimSpace(expanded)) {
			continue
		}
		if integers, ok := tryParseIntList(expanded); ok {
			rect = append(rect, integers...)
			continue
		}
		if looksLikeDisplayFlags(expanded) {
			if displayToken == nil {
				value := expanded
				displayToken = &value
			} else {
				joined := *displayToken + "@" + expanded
				displayToken = &joined
			}
			continue
		}
		if looksLikeResourceFlags(expanded) {
			if resourceToken == nil {
				value := expanded
				resourceToken = &value
			} else {
				joined := *resourceToken + "@" + expanded
				resourceToken = &joined
			}
			continue
		}
		if imageArgument == nil {
			copied := arg
			imageArgument = &copied
		} else {
			m.logOnce("set sprite '" + name + "' ignores extra argument '" + raw + "'")
		}
	}
	resources := 0
	if resourceToken != nil {
		resources = emontext.ParseSpriteResourceFlags(*resourceToken)
	}
	var full *bishopgfx.BsgImage
	imageFile := ""
	linked := false
	keepImage := false
	if imageArgument != nil {
		image := *imageArgument
		var file string
		if image.WasQuoted {
			file = image.Raw
		} else {
			file = m.valueOf(image.Raw, false).AsString()
		}
		imageFile = file
		if file != "" && !strings.EqualFold(file, "NULL") {
			source := m.context.Stage.Find(file)
			if source == nil {
				full = m.ResolveSpriteImage(file)
				if full == nil {
					m.logOnce("set sprite '" + name + "' image '" + file + "' was not found")
				}
			} else if source == sprite {
				keepImage = true
			} else {
				sprite.SetImageSource(source)
				sprite.SetImage(nil)
				m.sourceImages.Delete(name)
				linked = true
				full = source.ResolveImage()
			}
		}
	}
	if !linked && !keepImage {
		if full != nil {
			sprite.SetImageSource(nil)
			sprite.SetImage(full)
			sprite.SetAlpha(255)
			m.colorSprites.Remove(name)
			m.StoreSource(name, full)
		} else {
			full = m.SourceImage(name)
		}
	}
	if displayToken != nil {
		sprite.SetFlags(stage.EmonSpriteFlags(int(emontext.ParseSpriteDisplayFlags(*displayToken))))
		m.EnsureSelectOrder(sprite)
	}
	if len(rect) == 0 {
		if !linked && !keepImage {
			sprite.SetImage(full)
		}
		if !keepImage && full != nil {
			sprite.ClearSourceRect()
			sprite.SetSize(full.Width, full.Height)
			m.spriteRects.Delete(name)
		}
	} else {
		if len(rect) > 4 {
			m.logOnce("set sprite '" + name + "' ignores extra rectangle arguments")
		}
		if !keepImage {
			head := rect
			if len(rect) > 4 {
				head = rect[:4]
			}
			corners := ResolveRect(head, full)
			if len(rect) < 4 && full == nil {
				m.ApplyRect(sprite, corners)
			} else {
				m.applyRecordedRect(sprite, corners)
			}
		}
	}
	if !linked && sprite.Image() == nil && full == nil {
		corners := ResolveRect(rectHead(rect), nil)
		width := 1
		height := 1
		if len(rect) != 0 {
			width = max(1, corners.X2-corners.X1)
			height = max(1, corners.Y2-corners.Y1)
		}
		if resources&resourceClearBlack != 0 {
			blank := emongfx.Blank(width, height, 0, 0, 0)
			sprite.SetImage(blank)
			sprite.ClearSourceRect()
			sprite.SetSize(blank.Width, blank.Height)
			sprite.SetAlpha(255)
			m.colorSprites.Remove(name)
		} else if resources&resourceColor != 0 {
			highlight := emongfx.Blank(width, height, colorHighlightBlue, colorHighlightGreen, colorHighlightRed)
			sprite.SetImage(highlight)
			sprite.ClearSourceRect()
			sprite.SetSize(highlight.Width, highlight.Height)
			sprite.SetAlpha(colorHighlightAlpha)
			m.colorSprites.Add(name)
		}
	}
	m.spriteResources.Set(sprite.Name(), resources)
	if !linked && full != nil {
		m.applyImplicitMask(sprite, imageFile)
	}
	m.refreshLinkedFollowers()
	return EmonStepContinue
}

func rectHead(rect []int) []int {
	if len(rect) > 4 {
		return rect[:4]
	}
	return rect
}

func (m *Machine) onSetSprPos(statement *script.EmonStatement) EmonStep {
	name := m.ExpandToken(statement, 0)
	sprite := m.findSprite(name)
	if sprite == nil {
		m.logOnce("set sprite position '" + name + "' was not found")
		return EmonStepContinue
	}
	sprite.SetX(m.EvalInt(statement, 1, sprite.X()))
	sprite.SetY(m.EvalInt(statement, 2, sprite.Y()))
	return EmonStepContinue
}

func (m *Machine) onSetSprMsk(statement *script.EmonStatement) EmonStep {
	name := m.ExpandToken(statement, 0)
	sprite := m.findSprite(name)
	if sprite == nil {
		m.logOnce("set sprite mask '" + name + "' was not found")
		return EmonStepContinue
	}
	file := m.value(statement, 1).AsString()
	if file == "" {
		sprite.SetMask(nil)
		return EmonStepContinue
	}
	mask := m.ResolveSpriteImage(file)
	if mask == nil {
		m.logOnce("set sprite mask '" + file + "' was not found")
		return EmonStepContinue
	}
	sprite.SetMask(mask)
	return EmonStepContinue
}

func (m *Machine) onSetSprColor(statement *script.EmonStatement) EmonStep {
	name := m.ExpandToken(statement, 0)
	sprite := m.findSprite(name)
	if sprite == nil {
		m.logOnce("set sprite color '" + name + "' was not found")
		return EmonStepContinue
	}
	rgbIndex := m.colorArgumentIndex(statement)
	if rgbIndex >= statement.ArgumentCount() {
		return EmonStepContinue
	}
	amount := 256
	if statement.ArgumentCount() > rgbIndex+1 {
		amount = m.EvalInt(statement, rgbIndex+1, 256)
	}
	clamped := core.Clamp(amount, 0, 256)
	if m.colorSprites.Contains(name) {
		sprite.SetAlpha(clamped * 255 / 256)
		return EmonStepContinue
	}
	value := m.EvalInt(statement, rgbIndex, sprite.Tint()) & 0xFFFFFF
	sprite.SetTint(value)
	sprite.SetColorAmount(clamped)
	return EmonStepContinue
}

func (m *Machine) colorArgumentIndex(statement *script.EmonStatement) int32 {
	if statement.ArgumentCount() < 3 {
		return 1
	}
	if isColorModeToken(m.ExpandToken(statement, 1)) {
		return 2
	}
	return 1
}

func isColorModeToken(token string) bool {
	return strings.EqualFold(token, "COLOR") ||
		strings.EqualFold(token, "OVER") ||
		strings.EqualFold(token, "LIGHT") ||
		strings.EqualFold(token, "NULL") ||
		token == ""
}

func (m *Machine) onSetSprTrance(statement *script.EmonStatement) EmonStep {
	name := m.ExpandToken(statement, 0)
	sprite := m.findSprite(name)
	if sprite == nil {
		m.logOnce("set sprite transparency '" + name + "' was not found")
		return EmonStepContinue
	}
	sprite.SetAlpha(m.EvalInt(statement, 1, sprite.Alpha()))
	return EmonStepContinue
}

func (m *Machine) onSetSprRect(statement *script.EmonStatement) EmonStep {
	m.applySpriteRect(statement, false)
	return EmonStepContinue
}

func (m *Machine) onSetSprReRect(statement *script.EmonStatement) EmonStep {
	m.applySpriteRect(statement, false)
	return EmonStepContinue
}

func (m *Machine) onSetSprTileRect(statement *script.EmonStatement) EmonStep {
	m.applySpriteRect(statement, true)
	return EmonStepContinue
}

func (m *Machine) applySpriteRect(statement *script.EmonStatement, tile bool) {
	name := m.ExpandToken(statement, 0)
	sprite := m.findSprite(name)
	if sprite == nil {
		m.logOnce("set sprite rectangle '" + name + "' was not found")
		return
	}
	if special, ok := m.findRectToken(statement, 1, -1); ok {
		if IsDefaultRectToken(special) || IsMessageSizeRectToken(special) {
			m.ResetToFullImage(sprite)
		}
		if tile {
			sprite.SetFlags(sprite.Flags() | stage.EmonSpriteFlagsTile | stage.EmonSpriteFlagsClip)
		}
		return
	}
	rect := m.EvalIntegers(statement, 1)
	if len(rect) == 0 {
		return
	}
	if tile {
		sprite.SetFlags(sprite.Flags() | stage.EmonSpriteFlagsTile | stage.EmonSpriteFlagsClip)
	}
	source := sprite.ResolveImage()
	if source == nil {
		source = m.SourceImage(name)
	}
	m.applyRecordedRect(sprite, ResolveRect(rectHead(rect), source))
}

func (m *Machine) findRectToken(statement *script.EmonStatement, start int32, count int) (string, bool) {
	argc := statement.ArgumentCount()
	end := argc
	if count >= 0 && start+int32(count) < end {
		end = start + int32(count)
	}
	for i := start; i < end; i++ {
		arg := statement.Argument(i)
		var expanded string
		if arg.WasQuoted {
			expanded = arg.Raw
		} else {
			expanded = m.Lookup(arg.Raw).AsString()
		}
		for _, part := range strings.Split(expanded, ",") {
			token := strings.TrimSpace(part)
			if IsDefaultRectToken(token) || IsMessageSizeRectToken(token) {
				return token, true
			}
		}
	}
	return "", false
}

func (m *Machine) applyRecordedRect(sprite *stage.EmonSprite, rect EmonRect) {
	m.ApplyRect(sprite, rect)
	m.spriteRects.Set(sprite.Name(), strconv.Itoa(rect.X1)+","+strconv.Itoa(rect.Y1)+","+strconv.Itoa(rect.X2)+","+strconv.Itoa(rect.Y2))
}

func (m *Machine) onReRectDraw(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() < 6 {
		core.Logf("emon: re rectangle draw needs a destination, a rectangle and a source")
		return EmonStepContinue
	}
	destination := m.ExpandToken(statement, 0)
	sourceName := m.ExpandToken(statement, 5)
	var source *bishopgfx.BsgImage
	if found := m.findSprite(sourceName); found != nil {
		source = found.ResolveImage()
	}
	if source == nil {
		source = m.SourceImage(sourceName)
	}
	if source == nil {
		source = m.loadStageImage(sourceName)
	}
	if source == nil {
		m.logOnce("re rectangle draw source '" + sourceName + "' was not found")
		return EmonStepContinue
	}
	region := m.EvalIntegers(statement, 1)
	var corners EmonRect
	if _, ok := m.findRectToken(statement, 1, 4); ok {
		corners = EmonRect{X1: 0, Y1: 0, X2: source.Width, Y2: source.Height}
	} else {
		if len(region) < 4 {
			core.Logf("emon: re rectangle draw needs four rectangle numbers")
			return EmonStepContinue
		}
		corners = ResolveRect(region[:4], source)
	}
	left := core.Clamp(corners.X1, 0, max(0, source.Width-1))
	top := core.Clamp(corners.Y1, 0, max(0, source.Height-1))
	width := min(corners.X2-corners.X1, source.Width-left)
	height := min(corners.Y2-corners.Y1, source.Height-top)
	if width <= 0 || height <= 0 {
		core.Logf("emon: re rectangle draw region is empty")
		return EmonStepContinue
	}
	cropped := m.Crop(source, left, top, width, height)
	if cropped == nil {
		cropped = source
	}
	extra := m.EvalIntegers(statement, 6)
	finalImage := cropped
	if len(extra) >= 4 && (extra[2] != width || extra[3] != height) && extra[2] > 0 && extra[3] > 0 {
		finalImage = scaleImage(cropped, extra[2], extra[3])
	}
	sprite := m.context.Stage.GetOrCreate(destination, nil)
	sprite.SetImage(finalImage)
	sprite.ClearSourceRect()
	sprite.SetSize(finalImage.Width, finalImage.Height)
	m.StoreSource(destination, finalImage)
	sprite.SetImageSource(nil)
	m.spriteRects.Delete(destination)
	if len(extra) >= 2 {
		sprite.SetX(extra[0])
		sprite.SetY(extra[1])
	}
	m.refreshLinkedFollowers()
	return EmonStepContinue
}

func (m *Machine) loadStageImage(name string) *bishopgfx.BsgImage {
	if !isMainSprite(name) {
		return nil
	}
	return readCanvas(m.context.Composer.Current())
}

func (m *Machine) onGetSprPos(statement *script.EmonStatement) EmonStep {
	name := m.ExpandToken(statement, 0)
	sprite := m.findSprite(name)
	x := 0
	y := 0
	if sprite != nil {
		x = sprite.X()
		y = sprite.Y()
	} else {
		m.logOnce("get sprite position '" + name + "' was not found")
	}
	variableX := m.raw(statement, 1)
	if variableX != "" && !strings.EqualFold(variableX, "NULL") {
		m.SetVariable(variableX, script.EmonValueFromInt(int32(x)))
	}
	variableY := m.raw(statement, 2)
	if variableY != "" && !strings.EqualFold(variableY, "NULL") {
		m.SetVariable(variableY, script.EmonValueFromInt(int32(y)))
	}
	return EmonStepContinue
}

func (m *Machine) onGetOnSpriteName(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() == 0 {
		return EmonStepContinue
	}
	sprite := m.context.Stage.HitTest(m.pointerX, m.pointerY)
	name := "NULL"
	if sprite != nil {
		name = sprite.Name()
	}
	m.SetVariable(m.raw(statement, 0), script.EmonValueFromString(name))
	return EmonStepContinue
}

func (m *Machine) onSpriteSwitch(statement *script.EmonStatement) EmonStep {
	argc := statement.ArgumentCount()
	var i int32
	for i = 0; i+1 < argc; i += 2 {
		m.applySpriteSwitch(m.ExpandToken(statement, i), m.raw(statement, i+1))
	}
	if argc%2 != 0 {
		m.logOnce("sprite switch has a trailing name without flags")
	}
	return EmonStepContinue
}

func (m *Machine) applySpriteSwitch(name, flags string) {
	if name == "" {
		return
	}
	sprite := m.findSprite(name)
	if sprite == nil {
		m.logOnce("sprite switch '" + name + "' was not found")
		return
	}
	upper := strings.ToUpper(flags)
	if strings.Contains(upper, "DISPOFF") {
		sprite.SetVisible(false)
	} else if strings.Contains(upper, "DISPON") {
		sprite.SetVisible(true)
	}
	if strings.Contains(upper, "SELECTOFF") {
		sprite.SetFlags(sprite.Flags() &^ stage.EmonSpriteFlagsSelect)
	} else if strings.Contains(upper, "SELECTON") {
		sprite.SetFlags(sprite.Flags() | stage.EmonSpriteFlagsSelect)
		m.EnsureSelectOrder(sprite)
	}
	if strings.Contains(upper, "MTNOFF") {
		sprite.SetFlags(sprite.Flags() &^ stage.EmonSpriteFlagsMotion)
		delete(m.motions, strings.ToUpper(name))
	} else if strings.Contains(upper, "MTNON") {
		sprite.SetFlags(sprite.Flags() | stage.EmonSpriteFlagsMotion)
	}
	if strings.Contains(upper, "ANMOFF") {
		sprite.SetFlags(sprite.Flags() &^ stage.EmonSpriteFlagsAnimation)
	} else if strings.Contains(upper, "ANMON") {
		sprite.SetFlags(sprite.Flags() | stage.EmonSpriteFlagsAnimation)
	}
	if strings.Contains(upper, "BACKMOST") {
		m.context.Stage.Reorder(sprite, stage.EmonSpriteKindBack)
	} else if strings.Contains(upper, "TOPMOST") {
		m.context.Stage.Reorder(sprite, stage.EmonSpriteKindNone)
	} else if strings.Contains(upper, "MIDDLE_B") {
		m.context.Stage.Reorder(sprite, stage.EmonSpriteKindMiddleBack)
	} else if strings.Contains(upper, "MIDDLE_F") {
		m.context.Stage.Reorder(sprite, stage.EmonSpriteKindMiddleFront)
	}
	if strings.Contains(upper, "BENOT") || strings.Contains(upper, "SOON") {
		m.logOnce("sprite switch flag '" + flags + "' keeps draw order")
	}
}

func (m *Machine) onReLoadSprite(statement *script.EmonStatement) EmonStep {
	name := m.ExpandToken(statement, 0)
	if name == "" {
		core.Logf("emon: reload sprite name is empty")
		return EmonStepContinue
	}
	file := m.text(statement, 1)
	if file == "" || strings.EqualFold(file, "NULL") {
		return EmonStepContinue
	}
	existing := m.context.Stage.Find(name)
	source := m.context.Stage.Find(file)
	if source != nil {
		if source != existing {
			target := m.context.Stage.GetOrCreate(name, nil)
			target.SetImageSource(source)
			target.SetImage(nil)
			m.sourceImages.Delete(name)
		}
		m.refreshLinkedFollowers()
		return EmonStepContinue
	}
	image := m.ResolveSpriteImage(file)
	if image == nil {
		m.logOnce("reload sprite '" + name + "' file '" + file + "' was not found")
		return EmonStepContinue
	}
	sprite := m.context.Stage.GetOrCreate(name, nil)
	sprite.SetImageSource(nil)
	m.StoreSource(name, image)
	sprite.SetImage(image)
	if saved, ok := m.spriteRects.Get(name); ok {
		if rect, ok := tryParseIntList(saved); ok && len(rect) >= 4 {
			m.ApplyRect(sprite, EmonRect{X1: rect[0], Y1: rect[1], X2: rect[2], Y2: rect[3]})
		} else {
			sprite.ClearSourceRect()
			sprite.SetSize(image.Width, image.Height)
		}
	} else {
		sprite.ClearSourceRect()
		sprite.SetSize(image.Width, image.Height)
	}
	m.applyImplicitMask(sprite, file)
	m.refreshLinkedFollowers()
	return EmonStepContinue
}

func (m *Machine) refreshLinkedFollowers() {
	for _, follower := range m.context.Stage.All() {
		if follower.ImageSource() == nil || follower.Image() != nil {
			continue
		}
		resolved := follower.ResolveImage()
		if resolved == nil {
			continue
		}
		if saved, ok := m.spriteRects.Get(follower.Name()); ok {
			if numbers, ok := tryParseIntList(saved); ok && len(numbers) >= 4 {
				m.ApplyRect(follower, EmonRect{X1: numbers[0], Y1: numbers[1], X2: numbers[2], Y2: numbers[3]})
				continue
			}
		}
		follower.ClearSourceRect()
		follower.SetSize(resolved.Width, resolved.Height)
	}
}

func (m *Machine) applyImplicitMask(sprite *stage.EmonSprite, file string) {
	if file == "" {
		return
	}
	resources, ok := m.spriteResources.Get(sprite.Name())
	if !ok || resources&resourceMask == 0 {
		return
	}
	if indexOfToken(file, ".png") < 0 {
		return
	}
	if entry, _ := m.context.Archives.Locate(file); entry != nil {
		return
	}
	if mask := m.ResolveSpriteImage("m" + file); mask != nil {
		sprite.SetMask(mask)
	}
}

func (m *Machine) onClearSprite(statement *script.EmonStatement) EmonStep {
	name := m.ExpandToken(statement, 0)
	if name == "" {
		return EmonStepContinue
	}
	if isMainSprite(name) {
		rgb := 0
		if statement.ArgumentCount() > 1 {
			rgb = m.EvalInt(statement, 1, 0)
		}
		m.context.Composer.SetBackground(rgb&0xFF, (rgb>>8)&0xFF, (rgb>>16)&0xFF)
		m.context.Stage.Root().SetImage(nil)
		return EmonStepContinue
	}
	sprite := m.findSprite(name)
	if sprite == nil {
		m.logOnce("clear sprite '" + name + "' was not found")
		return EmonStepContinue
	}
	sprite.SetImage(nil)
	sprite.SetMask(nil)
	sprite.SetF3D(nil)
	sprite.ClearSourceRect()
	m.spriteRects.Delete(name)
	m.colorSprites.Remove(name)
	sprite.SetAlpha(255)
	sprite.SetColorAmount(0)
	if statement.ArgumentCount() > 1 {
		rgb := m.EvalInt(statement, 1, 0)
		if sprite.Width() > 0 && sprite.Height() > 0 {
			blank := emongfx.Blank(max(1, sprite.Width()), max(1, sprite.Height()), uint8(rgb&0xFF), uint8((rgb>>8)&0xFF), uint8((rgb>>16)&0xFF))
			sprite.SetImage(blank)
		}
	}
	m.refreshLinkedFollowers()
	return EmonStepContinue
}

func (m *Machine) onClearScreen(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() != 0 {
		m.logOnce("clear screen arguments are ignored")
	}
	m.context.Composer.SetBackground(0, 0, 0)
	return EmonStepContinue
}

func (m *Machine) onSwapScreen(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() != 0 {
		m.logOnce("swap screen arguments are ignored")
	}
	m.context.Composer.Snapshot()
	return EmonStepContinue
}

func (m *Machine) onSetEffectReady(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() > 1 {
		m.logOnce("set effect ready arguments are ignored")
	}
	m.context.Composer.Snapshot()
	return EmonStepContinue
}

func (m *Machine) onEffect(statement *script.EmonStatement) EmonStep {
	if m.isReplaying() {
		return EmonStepContinue
	}
	parts := []string{}
	for i := int32(1); i < statement.ArgumentCount(); i++ {
		arg := statement.Argument(i)
		if arg.WasQuoted {
			parts = append(parts, arg.Raw)
			continue
		}
		expanded := m.Lookup(arg.Raw).AsString()
		for _, part := range strings.Split(expanded, ",") {
			parts = append(parts, strings.TrimSpace(part))
		}
	}
	kind := stage.EmonEffectKindNone
	if len(parts) > 0 {
		kind = parseEffectKind(parts[0])
	}
	frames := 0
	if len(parts) > 1 {
		frames = m.parseIntPart(parts[1])
	}
	second := 0
	if len(parts) > 2 {
		second = m.parseIntPart(parts[2])
	}
	third := 0
	if len(parts) > 3 {
		third = m.parseIntPart(parts[3])
	}
	m.effect = m.context.Composer.BeginEffect(kind, max(0, frames), second, third)
	if m.skip || kind == stage.EmonEffectKindNone || frames <= 0 {
		m.context.Composer.SetEffect(nil)
		m.effect = nil
		return EmonStepContinue
	}
	m.context.Composer.SetEffect(m.effect)
	m.waitFrames = frames
	return EmonStepWait
}

func parseEffectKind(token string) stage.EmonEffectKind {
	switch strings.ToUpper(strings.TrimSpace(token)) {
	case "MIXBLEND":
		return stage.EmonEffectKindMixBlend
	case "MASKFADE":
		return stage.EmonEffectKindMaskFade
	case "LRSCROLL":
		return stage.EmonEffectKindLeftRightScroll
	case "TBSCROLL":
		return stage.EmonEffectKindTopBottomScroll
	}
	if strings.TrimSpace(token) != "" {
		core.Logf("emon: effect kind '%s' is not supported", token)
	}
	return stage.EmonEffectKindNone
}

func (m *Machine) onSetSprAnm(statement *script.EmonStatement) EmonStep {
	name := m.ExpandToken(statement, 0)
	if name == "" {
		return EmonStepContinue
	}
	sprite := m.findSprite(name)
	if sprite == nil {
		m.logOnce("set sprite animation '" + name + "' was not found")
		return EmonStepContinue
	}
	flags := stage.MotionParseAnimationFlags(m.raw(statement, 1))
	if flags != 0 {
		sprite.SetFlags(sprite.Flags() | stage.EmonSpriteFlagsAnimation)
	}
	m.spriteParams.Set(name+"#Anm", m.joinRaw(statement, 1))
	return EmonStepContinue
}

func (m *Machine) onSetSprMtn(statement *script.EmonStatement) EmonStep {
	name := m.ExpandToken(statement, 0)
	sprite := m.findSprite(name)
	if sprite == nil {
		m.logOnce("set sprite motion '" + name + "' was not found")
		return EmonStepContinue
	}
	token := m.raw(statement, 1)
	kind := stage.MotionParseFlags(token)
	if base := kind & motionBaseMask; base == motionF3DMove || base == motionF3DRollOn || base == motionF3DRollOff {
		return m.onSetSprMtnF3D(statement, sprite, token, kind)
	}
	shortForm := false
	var actionIndex int32 = 8
	if statement.ArgumentCount() < 8 {
		if kind&motionBaseMask == 0 || !m.value(statement, 2).IsInteger() {
			core.Logf("emon: set sprite motion needs delay, positions and duration")
			return EmonStepContinue
		}
		shortForm = true
		actionIndex = 3
	}
	delay := 0
	if !shortForm {
		delay = m.EvalInt(statement, 2, 0)
	}
	startX := sprite.X()
	startY := sprite.Y()
	if !shortForm {
		startX = m.EvalInt(statement, 3, sprite.X())
		startY = m.EvalInt(statement, 4, sprite.Y())
	}
	endX := startX
	endY := startY
	if !shortForm {
		endX = m.EvalInt(statement, 5, startX)
		endY = m.EvalInt(statement, 6, startY)
	}
	duration := m.EvalInt(statement, 2, 0)
	if !shortForm {
		duration = m.EvalInt(statement, 7, 0)
	}
	if !shortForm && kind&motionFromThis != 0 {
		startX = sprite.X()
		startY = sprite.Y()
	}
	if containsFold(token, "DISPON") {
		sprite.SetVisible(true)
	}
	startAction := ""
	completion := ""
	argc := statement.ArgumentCount()
	for i := actionIndex; i < argc; i++ {
		if i+1 >= argc {
			break
		}
		keyword := m.raw(statement, i)
		if strings.EqualFold(keyword, "ONACTION") {
			startAction = m.raw(statement, i+1)
			i++
		} else if strings.EqualFold(keyword, "OFFACTION") {
			completion = m.raw(statement, i+1)
			i++
		}
	}
	motion := stage.NewEmonSpriteMotion(sprite, kind, max(0, delay), startX, startY, endX, endY, max(0, duration), kind&stage.MotionFadeInFlag != 0, kind&stage.MotionFadeOutFlag != 0)
	m.replaceMotions(name, motion, startAction, completion)
	return EmonStepContinue
}

func (m *Machine) onSetSprMtnF3D(statement *script.EmonStatement, sprite *stage.EmonSprite, token string, kind int) EmonStep {
	name := sprite.Name()
	if containsFold(token, "DISPON") {
		sprite.SetVisible(true)
	}
	delay := max(0, m.EvalInt(statement, 2, 0))
	fadeIn := kind&stage.MotionFadeInFlag != 0
	fadeOut := kind&stage.MotionFadeOutFlag != 0
	baseKind := kind & motionBaseMask
	move := make([]float64, 12)
	roll := make([]float64, 8)
	rollToken := ""
	rag := 0.0
	duration := 0
	var actionStartIndex int32
	if baseKind == motionF3DMove {
		for i := 0; i < 12; i++ {
			move[i] = m.evalDoubleArg(statement, int32(3+i))
		}
		duration = max(0, m.EvalInt(statement, 15, 0))
		actionStartIndex = 16
	} else {
		for i := 0; i < 8; i++ {
			roll[i] = m.evalDoubleArg(statement, int32(3+i))
		}
		if statement.ArgumentCount() > 11 {
			rollToken = m.raw(statement, 11)
		}
		if containsFold(rollToken, "SETRAG") {
			rag = m.evalDoubleArg(statement, 12)
		}
		duration = rollDuration(roll[7], rag)
		actionStartIndex = 11
	}
	startAction := ""
	completion := ""
	argc := statement.ArgumentCount()
	for i := actionStartIndex; i < argc; i++ {
		if i+1 >= argc {
			break
		}
		keyword := m.raw(statement, i)
		if strings.EqualFold(keyword, "ONACTION") {
			startAction = m.raw(statement, i+1)
			i++
		} else if strings.EqualFold(keyword, "OFFACTION") {
			completion = m.raw(statement, i+1)
			i++
		}
	}
	if baseKind != motionF3DMove && completion != "" && duration <= 0 {
		duration = max(1, delay)
		rag = roll[7] * float64(duration)
	}
	motion := stage.NewEmonSpriteMotion(sprite, kind, delay, sprite.X(), sprite.Y(), sprite.X(), sprite.Y(), duration, fadeIn, fadeOut)
	if baseKind == motionF3DMove {
		motion.SetF3DMove(move)
	} else {
		motion.SetF3DRoll(roll)
		motion.SetF3DRound(containsFold(rollToken, "ROUND"))
		motion.SetF3DStop(containsFold(rollToken, "STOP"))
		motion.SetF3DRag(rag)
	}
	if sprite.F3D() == nil {
		m.logOnce("set sprite motion '" + name + "' has no F3D state; motion stays 2D")
	} else {
		motion.SetF3DTarget(sprite.F3D())
	}
	m.addMotion(name, motion, startAction, completion)
	return EmonStepContinue
}

func rollDuration(radius, rag float64) int {
	if rag == 0.0 || radius == 0.0 {
		return 0
	}
	frames := math.Abs(rag) / math.Abs(radius)
	if frames >= 2147483647 {
		return 2147483647
	}
	return max(1, int(math.Ceil(frames)))
}

func (m *Machine) onSetSprMsg(statement *script.EmonStatement) EmonStep {
	name := m.ExpandToken(statement, 0)
	if name == "" {
		return EmonStepContinue
	}
	m.spriteParams.Set(name+"#Msg", m.joinRaw(statement, 1))
	m.applySpriteTextSetting(name, statement, true)
	m.setMessageBoxSprite(name)
	return EmonStepContinue
}

func (m *Machine) onSetSprF3D(statement *script.EmonStatement) EmonStep {
	name := m.ExpandToken(statement, 0)
	if name == "" {
		return EmonStepContinue
	}
	sprite := m.findSprite(name)
	if sprite == nil {
		m.logOnce("set sprite F3D '" + name + "' was not found")
		return EmonStepContinue
	}
	subCmd := m.raw(statement, 1)
	if subCmd == "" {
		return EmonStepContinue
	}
	setCamera := false
	setAngle := false
	setScale := false
	setVector := false
	release := false
	for _, part := range strings.Split(subCmd, "@") {
		switch strings.ToUpper(strings.TrimSpace(part)) {
		case "":
		case "RELEASE":
			release = true
		case "SETCAMERA":
			setCamera = true
		case "ANGLE":
			setAngle = true
		case "SCALE":
			setScale = true
		case "VECTOR":
			setVector = true
		}
	}
	if release {
		sprite.SetF3D(nil)
		return EmonStepContinue
	}
	if !(setCamera || setAngle || setScale || setVector) {
		return EmonStepContinue
	}
	if sprite.F3D() == nil {
		sprite.SetF3D(stage.NewEmonF3DState())
	}
	argc := int(statement.ArgumentCount())
	argIndex := 2
	if setCamera {
		sprite.F3D().SetCamera(m.evalDoubleArg(statement, int32(argIndex)), m.evalDoubleArg(statement, int32(argIndex+1)), m.evalDoubleArg(statement, int32(argIndex+2)))
		argIndex += 3
	}
	if setAngle {
		values := make([]float64, 12)
		available := min(12, max(0, argc-argIndex))
		for i := 0; i < available; i++ {
			values[i] = m.evalDoubleArg(statement, int32(argIndex+i))
		}
		sprite.F3D().SetAngle(values)
		argIndex += 12
	}
	if setScale {
		if argc-argIndex >= 6 {
			values := make([]float64, 6)
			for i := 0; i < 6; i++ {
				values[i] = m.evalDoubleArg(statement, int32(argIndex+i))
			}
			sprite.F3D().SetScale(values)
		}
		argIndex += 6
	}
	if setVector {
		values := make([]float64, 13)
		available := min(13, max(0, argc-argIndex))
		for i := 0; i < available; i++ {
			values[i] = m.evalDoubleArg(statement, int32(argIndex+i))
		}
		sprite.F3D().SetVector(values)
		argIndex += 13
	}
	return EmonStepContinue
}

func (m *Machine) evalDoubleArg(statement *script.EmonStatement, index int32) float64 {
	return float64(m.EvalInt(statement, index, 0))
}

func (m *Machine) onSetMsgHit(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() > 1 {
		m.logOnce("set message hit arguments are ignored")
	}
	m.setMessageHitSprite(m.ExpandToken(statement, 0))
	m.applyMessageIndicators(m.step)
	return EmonStepContinue
}

func (m *Machine) onSetMsgPage(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() > 1 {
		m.logOnce("set message page arguments are ignored")
	}
	m.setMessagePageSprite(m.ExpandToken(statement, 0))
	m.applyMessageIndicators(m.step)
	return EmonStepContinue
}
func (m *Machine) onSetSprText(statement *script.EmonStatement) EmonStep {
	name := m.ExpandToken(statement, 0)
	if name == "" {
		return EmonStepContinue
	}
	m.applySpriteTextSetting(name, statement, false)
	return EmonStepContinue
}

func (m *Machine) applySpriteTextSetting(name string, statement *script.EmonStatement, message bool) {
	sprite := m.context.Stage.Find(name)
	if sprite == nil {
		m.logOnce("set sprite text '" + name + "' was not found")
		return
	}
	tokens := m.textArguments(statement, 1)
	if len(tokens) < 4 {
		m.logOnce("set sprite text '" + name + "' needs a position, a font and a size")
		return
	}
	x := m.parseIntPart(tokens[0])
	y := m.parseIntPart(tokens[1])
	family := unquote(tokens[2])
	size := m.parseIntPart(tokens[3])
	boxWidth := 0
	boxHeight := 0
	if len(tokens) >= 6 {
		boxWidth = m.parseIntPart(tokens[4])
		boxHeight = m.parseIntPart(tokens[5])
	} else {
		sourceWidth := sprite.Width()
		if sourceWidth <= 0 {
			if resolved := sprite.ResolveImage(); resolved != nil {
				sourceWidth = resolved.Width
			}
		}
		sourceHeight := sprite.Height()
		if sourceHeight <= 0 {
			if resolved := sprite.ResolveImage(); resolved != nil {
				sourceHeight = resolved.Height
			}
		}
		boxWidth = max(1, sourceWidth-2*x)
		boxHeight = max(1, sourceHeight-2*y)
	}
	flags := emontext.EmonSpriteProportion
	if len(tokens) >= 7 {
		flags = emontext.ParseSpriteTextFlags(tokens[6])
	}
	if message {
		flags = m.mergeMessageFontFlags(flags)
	}
	setting := emontext.NewEmonSpriteTextSetting(name, x, y, family, size, boxWidth, boxHeight)
	setting.SetFlags(flags)
	if len(tokens) >= 8 {
		setting.SetColor(m.parseColor(tokens[7]))
	} else {
		setting.SetColor(0xFFFFFF)
	}
	if len(tokens) >= 9 {
		setting.SetShadowColor(m.parseColor(tokens[8]))
	} else {
		setting.SetShadowColor(0)
	}
	if len(tokens) >= 10 {
		setting.SetBlockColor(m.parseColor(tokens[9]))
	} else {
		setting.SetBlockColor(0)
	}
	if len(tokens) >= 11 {
		setting.SetLineHeightOverride(m.parseIntPart(tokens[10]))
	} else {
		setting.SetLineHeightOverride(0)
	}
	sprite.SetTextSetting(setting)
	if sprite.Text() == nil {
		sprite.SetText(emontext.NewEmonSpriteText())
	}
}

func (m *Machine) mergeMessageFontFlags(own int) int {
	registered := m.messageBoxSprite()
	if registered != "" {
		if previous := m.context.Stage.Find(registered); previous != nil && previous.TextSetting() != nil {
			return emontext.InheritMessageFlags(own, previous.TextSetting().Flags())
		}
	}
	systemVariables := m.context.SystemVariables
	stored := systemVariables.Lookup(emontext.EmonSpriteSystemFontFlag)
	if !stored.IsIdentifier() {
		value := stored.AsInt()
		if value < 0 {
			systemVariables.Set(emontext.EmonSpriteSystemFontFlag, script.EmonValueFromInt(value&0x7FFFFFFF))
		} else {
			own = emontext.MergeSystemFontFlag(own, int(value))
		}
	}
	systemVariables.Set(emontext.EmonSpriteSystemFontFlag, script.EmonValueFromInt(int32(own&emontext.EmonSpriteMessageFlagMask)))
	return own
}

func (m *Machine) onSetSprTextFont(statement *script.EmonStatement) EmonStep {
	name := m.ExpandToken(statement, 0)
	if name == "" {
		return EmonStepContinue
	}
	m.spriteParams.Set(name+"#Font", m.text(statement, 1))
	return EmonStepContinue
}

func (m *Machine) onSetSprTextFontFlag(statement *script.EmonStatement) EmonStep {
	name := m.ExpandToken(statement, 0)
	if name == "" {
		return EmonStepContinue
	}
	tokens := m.textArguments(statement, 1)
	flags := ""
	if len(tokens) >= 1 {
		flags = tokens[0]
	}
	m.spriteParams.Set(name+"#FontFlag", flags)
	sprite := m.context.Stage.Find(name)
	if sprite == nil || sprite.TextSetting() == nil {
		m.logOnce("set sprite text flag '" + name + "' has no text box")
		return EmonStepContinue
	}
	sprite.TextSetting().SetFlags(emontext.ParseSpriteTextFlags(flags))
	return EmonStepContinue
}

func (m *Machine) onSetSprTextColor(statement *script.EmonStatement) EmonStep {
	name := m.raw(statement, 0)
	if name == "" {
		m.logOnce("set sprite text colour needs a sprite")
		return EmonStepContinue
	}
	sprite := m.context.Stage.Find(name)
	if sprite == nil {
		m.logOnce("set sprite text colour '" + name + "' was not found")
		return EmonStepContinue
	}
	setting := sprite.TextSetting()
	if setting == nil {
		m.logOnce("set sprite text colour '" + name + "' has no text box")
		return EmonStepContinue
	}
	setting.SetColor(m.spriteTextColor(statement, 1, 0xFFFFFF))
	setting.SetShadowColor(m.spriteTextColor(statement, 2, 0))
	setting.SetBlockColor(m.spriteTextColor(statement, 3, 0))
	return EmonStepContinue
}

func (m *Machine) onSetSpriteMsgSelect(statement *script.EmonStatement) EmonStep {
	keyword := m.raw(statement, 1)
	var mode int
	if !emontext.TryParseMessageSelect(keyword, &mode) {
		m.logOnce("sprite message select '" + keyword + "' is not supported")
		return EmonStepContinue
	}
	name := m.raw(statement, 0)
	if name == "" {
		m.logOnce("sprite message select needs a sprite")
		return EmonStepContinue
	}
	sprite := m.context.Stage.Find(name)
	if sprite == nil || sprite.TextSetting() == nil {
		m.logOnce("sprite message select '" + name + "' has no text box")
		return EmonStepContinue
	}
	sprite.TextSetting().SetMessageSelectMode(mode)
	m.SetVariable("SprMsgSelectNumber", script.EmonValueFromInt(0))
	return EmonStepContinue
}

func (m *Machine) spriteTextColor(statement *script.EmonStatement, index int32, fallback int) int {
	arg := statement.Argument(index)
	if arg.Raw == "" {
		return fallback
	}
	value := m.EvalInt(statement, index, fallback)
	color := fallback
	if value != 0 || isNumericToken(arg.Raw) {
		color = value
	}
	if color == 0x00FF00 {
		return 0x00FE00
	}
	return color
}

func isNumericToken(token string) bool {
	part := strings.TrimSpace(token)
	if part == "" {
		return false
	}
	if len(part) > 2 && part[0] == '0' && (part[1] == 'x' || part[1] == 'X') {
		digits := part[2:]
		if digits == "" {
			return false
		}
		_, err := strconv.ParseUint(digits, 16, 32)
		return err == nil
	}
	_, ok := parseIntInvariant(part)
	return ok
}

func (m *Machine) onSprWriteText(statement *script.EmonStatement) EmonStep {
	name := m.ExpandToken(statement, 0)
	if name == "" {
		return EmonStepContinue
	}
	sprite := m.context.Stage.Find(name)
	if sprite == nil || sprite.TextSetting() == nil {
		m.logOnce("spr write text '" + name + "' has no text box")
		return EmonStepContinue
	}
	text := sprite.Text()
	if text == nil {
		text = emontext.NewEmonSpriteText()
	}
	sprite.SetText(text)
	text.Clear()
	if statement.ArgumentCount() >= 2 {
		for _, segment := range strings.Split(m.text(statement, 1), "\\n") {
			text.Append(segment)
		}
	}
	sprite.InvalidateText()
	m.SetSpriteText(name, text.Content())
	return EmonStepContinue
}

func (m *Machine) onSetEdit(statement *script.EmonStatement) EmonStep {
	if m.isReplaying() {
		return EmonStepContinue
	}
	name := m.raw(statement, 0)
	if name == "" {
		m.logOnce("set edit needs a sprite")
		return EmonStepContinue
	}
	sprite := m.context.Stage.Find(name)
	if sprite == nil {
		m.logOnce("set edit '" + name + "' was not found")
		return EmonStepContinue
	}
	if sprite.TextSetting() == nil {
		m.logOnce("set edit '" + name + "' has no text box")
		return EmonStepContinue
	}
	variable := m.raw(statement, 1)
	if variable == "" {
		variable = name
	}
	current := m.messageSpriteText(name, sprite, false)
	m.editSprite = name
	m.editVariable = variable
	prompt := current
	if prompt == "" {
		prompt = name
	}
	m.setPrompt(prompt, variable, false)
	return EmonStepAwaitName
}

func (m *Machine) refreshEditSpriteText() {
	if m.editSprite == "" || m.hasPrompt {
		return
	}
	name := m.editSprite
	variable := m.editVariable
	m.editSprite = ""
	m.editVariable = ""
	sprite := m.context.Stage.Find(name)
	if sprite == nil || sprite.TextSetting() == nil {
		return
	}
	value := m.Lookup(variable).AsString()
	if value == "" || value == m.messageSpriteText(name, sprite, false) {
		return
	}
	text := sprite.Text()
	if text == nil {
		text = emontext.NewEmonSpriteText()
	}
	sprite.SetText(text)
	text.SetContent(value)
	sprite.InvalidateText()
	m.SetSpriteText(name, text.Content())
}

func (m *Machine) textArguments(statement *script.EmonStatement, start int32) []string {
	tokens := []string{}
	for index := start; index < statement.ArgumentCount(); index++ {
		arg := statement.Argument(index)
		if arg.Raw == "" {
			continue
		}
		if arg.WasQuoted {
			tokens = append(tokens, arg.Raw)
			continue
		}
		expanded := strings.TrimSpace(m.Lookup(arg.Raw).AsString())
		if expanded == "" {
			continue
		}
		if strings.Contains(expanded, ",") {
			for _, part := range strings.Split(expanded, ",") {
				trimmed := strings.TrimSpace(part)
				if trimmed != "" {
					tokens = append(tokens, trimmed)
				}
			}
			continue
		}
		tokens = append(tokens, expanded)
	}
	return tokens
}

func (m *Machine) parseColor(token string) int {
	return int(m.valueOf(token, false).AsInt()) & 0xFFFFFF
}

func unquote(token string) string {
	if len(token) >= 2 && token[0] == '"' && token[len(token)-1] == '"' {
		return token[1 : len(token)-1]
	}
	return token
}

func (m *Machine) onUnrag(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() != 0 {
		m.logOnce("unrag arguments are ignored")
	} else {
		m.logOnce("unrag is not supported")
	}
	return EmonStepContinue
}

func (m *Machine) onSetCursol(statement *script.EmonStatement) EmonStep {
	m.setCursorSprite(m.ExpandToken(statement, 0))
	m.setCursorX(m.EvalInt(statement, 1, 0))
	m.setCursorY(m.EvalInt(statement, 2, 0))
	return EmonStepContinue
}

func (m *Machine) onMovie(statement *script.EmonStatement) EmonStep {
	if m.isReplaying() {
		return EmonStepContinue
	}
	file := ""
	if statement.ArgumentCount() != 0 {
		file = m.text(statement, 0)
	}
	if file == "" {
		m.movie = ""
		m.hasMovie = false
	} else {
		m.movie = file
		m.hasMovie = true
	}
	return EmonStepContinue
}

func (m *Machine) onSetMaskSelects(statement *script.EmonStatement) EmonStep {
	m.spriteParams.Set("MaskSelects", m.joinRaw(statement, 0))
	return EmonStepContinue
}

func (m *Machine) onClick(statement *script.EmonStatement) EmonStep {
	if m.isReplaying() {
		return EmonStepContinue
	}
	if statement.ArgumentCount() != 0 {
		m.logOnce("click arguments are ignored")
	}
	m.waitFrames = 1
	m.clickWaiting = true
	return EmonStepWait
}

func (m *Machine) onAppearMenu(statement *script.EmonStatement) EmonStep {
	kind := m.raw(statement, 0)
	if strings.EqualFold(kind, "HERONAME") {
		m.setPrompt(kind, "主人公名前", false)
		return EmonStepAwaitName
	}
	if strings.EqualFold(kind, "AUTOSETTING") {
		return EmonStepContinue
	}
	m.logOnce("menu '" + kind + "' is not supported")
	return EmonStepContinue
}

func (m *Machine) joinRaw(statement *script.EmonStatement, start int32) string {
	parts := []string{}
	for i := start; i < statement.ArgumentCount(); i++ {
		parts = append(parts, statement.Argument(i).Raw)
	}
	return strings.Join(parts, ",")
}

func tryParseIntList(text string) ([]int, bool) {
	integers := []int{}
	for _, part := range strings.Split(text, ",") {
		value, ok := parseIntInvariant(strings.TrimSpace(part))
		if !ok {
			return nil, false
		}
		integers = append(integers, int(value))
	}
	if len(integers) == 0 {
		return nil, false
	}
	return integers, true
}

func looksLikeDisplayFlags(token string) bool {
	upper := strings.ToUpper(token)
	return strings.Contains(upper, "DISPON") || strings.Contains(upper, "DISPOFF") || strings.Contains(upper, "CLIP") ||
		strings.Contains(upper, "SELECT") || strings.Contains(upper, "EFFECTONLY") || strings.Contains(upper, "DRAWME") ||
		strings.Contains(upper, "CLEARME") || strings.Contains(upper, "BACK") || strings.Contains(upper, "TILE")
}

func looksLikeResourceFlags(token string) bool {
	upper := strings.ToUpper(token)
	return strings.Contains(upper, "FILE") || strings.Contains(upper, "SPRITE") || strings.Contains(upper, "NEW") ||
		strings.Contains(upper, "CLEARBLACK") || strings.Contains(upper, "CLEARNULL") || strings.Contains(upper, "MASK") ||
		strings.Contains(upper, "TRANCE") || strings.Contains(upper, "NOTEREA") || strings.Contains(upper, "COLOR") ||
		strings.Contains(upper, "MUST")
}

func containsFold(text, part string) bool {
	return strings.Contains(strings.ToUpper(text), strings.ToUpper(part))
}

func (m *Machine) pumpMotions() {
	m.refreshEditSpriteText()
	if len(m.motions) == 0 {
		return
	}
	now := time.Now()
	var finished []ActiveMotion
	var started []ActiveMotion
	var moved []*stage.EmonSprite
	var emptied []string
	for key, active := range m.motions {
		for i := len(active) - 1; i >= 0; i-- {
			item := active[i]
			motion := item.Motion
			delta := now.Sub(item.Touched).Seconds()
			active[i].Touched = now
			complete := false
			func() {
				defer func() {
					if recovered := recover(); recovered != nil {
						core.Logf("emon: motion '%s' failed: %v", key, recovered)
						complete = true
					}
				}()
				if motion.TakeStart() {
					started = append(started, item)
				}
				owner := motion.Owner()
				beforeX := owner.X()
				beforeY := owner.Y()
				complete = motion.Advance(delta)
				if owner.X() != beforeX || owner.Y() != beforeY {
					seen := false
					for _, existing := range moved {
						if existing == owner {
							seen = true
							break
						}
					}
					if !seen {
						moved = append(moved, owner)
					}
				}
			}()
			if !complete {
				continue
			}
			active = append(active[:i], active[i+1:]...)
			m.motions[key] = active
			finished = append(finished, item)
		}
		if len(m.motions[key]) == 0 {
			emptied = append(emptied, key)
		}
	}
	if len(emptied) != 0 {
		for _, key := range emptied {
			if active, ok := m.motions[key]; ok && len(active) == 0 {
				delete(m.motions, key)
			}
		}
	}
	if len(moved) != 0 {
		for _, sprite := range moved {
			m.fireMoveTriggers(sprite)
		}
	}
	if len(started) != 0 {
		for _, item := range started {
			if item.Start != "" {
				m.executeRunAction(item.Start, 0)
			}
		}
	}
	if len(finished) == 0 {
		return
	}
	for _, item := range finished {
		if item.Completion != "" {
			m.executeRunAction(item.Completion, 0)
		}
	}
}

func (m *Machine) replaceMotions(name string, motion *stage.EmonSpriteMotion, start, completion string) {
	m.motions[strings.ToUpper(name)] = []ActiveMotion{{Motion: motion, Start: start, Completion: completion, Touched: time.Now()}}
}

func (m *Machine) addMotion(name string, motion *stage.EmonSpriteMotion, start, completion string) {
	key := strings.ToUpper(name)
	m.motions[key] = append(m.motions[key], ActiveMotion{Motion: motion, Start: start, Completion: completion, Touched: time.Now()})
}

func (m *Machine) clearMotions() {
	m.motions = make(map[string][]ActiveMotion)
}
