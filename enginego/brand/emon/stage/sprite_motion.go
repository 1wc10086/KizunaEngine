package stage

import (
	"math"
	"strings"
)

const (
	MotionStraight    = 1
	MotionAccell      = 2
	MotionFadeInFlag  = 0x40000
	MotionFadeOutFlag = 0x80000

	motionFramesPerSecond = 60.0
	motionOpaque          = 256
	fullRollDegrees       = 360.0
	minimumRollDegrees    = 0.0
	minimumRollFrames     = 1.0
	maximumRollFrames     = 360.0
)

type EmonSpriteMotion struct {
	owner    *EmonSprite
	kind     int
	delay    int
	duration int
	startX   int
	startY   int
	endX     int
	endY     int
	fadeIn   bool
	fadeOut  bool

	alphaTarget  int
	elapsed      float64
	pendingStart bool
	started      bool

	f3dScratch [16]float64

	f3dTarget *EmonF3DState
	f3dMove   []float64
	f3dRoll   []float64
	f3dRag    float64
	f3dRound  bool
	f3dStop   bool

	rollAngle    float64
	rollBudget   float64
	rollFrames   float64
	rollResolved bool

	isComplete bool
}

func NewEmonSpriteMotion(owner *EmonSprite, kind, delay, startX, startY, endX, endY, duration int, fadeIn, fadeOut bool) *EmonSpriteMotion {
	motion := &EmonSpriteMotion{
		owner:    owner,
		kind:     kind,
		delay:    delay,
		startX:   startX,
		startY:   startY,
		endX:     endX,
		endY:     endY,
		duration: duration,
		fadeIn:   fadeIn,
		fadeOut:  fadeOut,
	}

	target := owner.Alpha()
	if target >= 255 {
		motion.alphaTarget = motionOpaque
	} else {
		motion.alphaTarget = int(math.RoundToEven(float64(target) * motionOpaque / 255.0))
	}
	if motion.alphaTarget == 0 {
		motion.alphaTarget = motionOpaque
	}

	return motion
}

func (m *EmonSpriteMotion) Owner() *EmonSprite {
	return m.owner
}

func (m *EmonSpriteMotion) Kind() int {
	return m.kind
}

func (m *EmonSpriteMotion) Delay() int {
	return m.delay
}

func (m *EmonSpriteMotion) Duration() int {
	return m.duration
}

func (m *EmonSpriteMotion) StartX() int {
	return m.startX
}

func (m *EmonSpriteMotion) StartY() int {
	return m.startY
}

func (m *EmonSpriteMotion) EndX() int {
	return m.endX
}

func (m *EmonSpriteMotion) EndY() int {
	return m.endY
}

func (m *EmonSpriteMotion) FadeIn() bool {
	return m.fadeIn
}

func (m *EmonSpriteMotion) FadeOut() bool {
	return m.fadeOut
}

func (m *EmonSpriteMotion) F3DTarget() *EmonF3DState {
	return m.f3dTarget
}

func (m *EmonSpriteMotion) SetF3DTarget(value *EmonF3DState) {
	m.f3dTarget = value
}

func (m *EmonSpriteMotion) F3DMove() []float64 {
	return m.f3dMove
}

func (m *EmonSpriteMotion) SetF3DMove(value []float64) {
	m.f3dMove = value
}

func (m *EmonSpriteMotion) F3DRoll() []float64 {
	return m.f3dRoll
}

func (m *EmonSpriteMotion) SetF3DRoll(value []float64) {
	m.f3dRoll = value
}

func (m *EmonSpriteMotion) F3DRag() float64 {
	return m.f3dRag
}

func (m *EmonSpriteMotion) SetF3DRag(value float64) {
	m.f3dRag = value
}

func (m *EmonSpriteMotion) F3DRound() bool {
	return m.f3dRound
}

func (m *EmonSpriteMotion) SetF3DRound(value bool) {
	m.f3dRound = value
}

func (m *EmonSpriteMotion) F3DStop() bool {
	return m.f3dStop
}

func (m *EmonSpriteMotion) SetF3DStop(value bool) {
	m.f3dStop = value
}

func (m *EmonSpriteMotion) Elapsed() float64 {
	return m.elapsed
}

func (m *EmonSpriteMotion) BaseKind() int {
	return m.kind & 0xFFFF
}

func (m *EmonSpriteMotion) IsComplete() bool {
	return m.isComplete
}

func (m *EmonSpriteMotion) TakeStart() bool {
	if !m.pendingStart {
		return false
	}

	m.pendingStart = false
	return true
}

func MotionParseFlags(token string) int {
	if len(token) == 0 {
		return 0
	}

	kind := 0
	if motionContains(token, "STRAIGHT") {
		kind = 1
	}
	if motionContains(token, "ACCELL") {
		kind = 2
	}
	if motionContains(token, "SIN") {
		kind = 3
	}
	if motionContains(token, "QUAKE") {
		kind = 4
	}
	if motionContains(token, "BLINK") {
		kind = 5
	}
	if motionContains(token, "TIMER") {
		kind = 6
	}
	if motionContains(token, "ROLLON") {
		kind = 7
	}
	if motionContains(token, "ROLLOFF") {
		kind = 8
	}
	if motionContains(token, "TURN") {
		kind = 10
	}
	if motionContains(token, "COLOR") {
		kind = 11
	}
	if motionContains(token, "F3DROLLON") {
		kind = 13
	}
	if motionContains(token, "F3DROLLOFF") {
		kind = 14
	}
	if motionContains(token, "F3DMOVE") {
		kind = 15
	}
	if motionContains(token, "DISPON") {
		kind |= 0x10000
	}
	if motionContains(token, "TILELOOP") {
		kind |= 0x100000
	}
	if motionContains(token, "TIMES") {
		kind |= 0x20000
	}
	if motionContains(token, "FADEIN") {
		kind |= MotionFadeInFlag
	}
	if motionContains(token, "FADEOUT") {
		kind |= MotionFadeOutFlag
	}
	if motionContains(token, "QUARITY") {
		kind |= 0x200000
	}
	if motionContains(token, "FROMTHIS") {
		kind |= 0x400000
	}
	if motionContains(token, "NOSTOP") {
		kind |= 0x20000000
	}

	return kind
}

func MotionParseAnimationFlags(token string) int {
	if len(token) == 0 {
		return 0
	}

	flags := 0
	if motionContains(token, "ANMON") {
		flags = 1
	}
	if motionContains(token, "ONETIME") {
		flags |= 2
	}

	return flags
}

func (m *EmonSpriteMotion) Advance(deltaSeconds float64) bool {
	if m.isComplete {
		return true
	}

	m.elapsed = math.Max(0, m.elapsed+deltaSeconds)
	frames := m.elapsed * motionFramesPerSecond
	if frames < float64(m.delay) {
		return false
	}

	if !m.started {
		m.started = true
		m.pendingStart = true
	}

	progress := 1.0
	if m.duration > 0 {
		progress = math.Min(1.0, (frames-float64(m.delay))/float64(m.duration))
	}

	if m.BaseKind() == 15 {
		m.advanceF3DMove(progress)
		m.applyFade(progress)
		if progress < 1.0 {
			return false
		}

		m.settleFade()
		m.isComplete = true
		return true
	}

	if m.BaseKind() == 13 || m.BaseKind() == 14 {
		m.advanceF3DRoll(deltaSeconds)
		rollProgress := m.rollProgress(frames - float64(m.delay))
		m.applyFade(rollProgress)
		if rollProgress < 1.0 {
			return false
		}

		m.settleFade()
		m.isComplete = true
		return true
	}

	shaped := progress
	if m.BaseKind() == MotionAccell {
		shaped = (2.0 * progress) - (progress * progress)
	}

	m.owner.SetX(m.startX + int(math.RoundToEven(float64(m.endX-m.startX)*shaped)))
	m.owner.SetY(m.startY + int(math.RoundToEven(float64(m.endY-m.startY)*shaped)))
	m.applyFade(progress)

	if progress < 1.0 {
		return false
	}

	m.settleFade()
	m.isComplete = true
	return true
}

func (m *EmonSpriteMotion) applyFade(progress float64) {
	if m.fadeOut {
		m.owner.SetAlpha(toMotionAlpha(m.alphaTarget - int(float64(m.alphaTarget)*progress)))
	} else if m.fadeIn {
		m.owner.SetAlpha(toMotionAlpha(int(float64(m.alphaTarget) * progress)))
	}
}

func (m *EmonSpriteMotion) settleFade() {
	if m.fadeOut {
		m.owner.SetAlpha(0)
		m.owner.SetVisible(false)
	} else if m.fadeIn {
		m.owner.SetAlpha(toMotionAlpha(m.alphaTarget))
	}
}

func toMotionAlpha(value int) int {
	scaled := math.Max(0, float64(value)) * 255.0 / motionOpaque
	rounded := int(math.RoundToEven(scaled))
	return min(255, rounded)
}

func motionContains(token, value string) bool {
	return strings.Contains(strings.ToUpper(token), value)
}

func (m *EmonSpriteMotion) advanceF3DMove(progress float64) {
	if len(m.f3dMove) < 12 {
		return
	}

	if m.f3dTarget == nil {
		return
	}

	move := m.f3dMove
	p0x := move[0]
	p0y := move[1]
	p0z := move[2]
	t0x := move[3]
	t0y := move[4]
	t0z := move[5]
	p1x := move[6]
	p1y := move[7]
	p1z := move[8]
	t1x := move[9]
	t1y := move[10]
	t1z := move[11]

	t := progress
	tSquared := t * t
	tCubed := tSquared * t

	cx := 2.0*p0x - 2.0*p1x + t0x + t1x
	bx := 3.0*p1x - 3.0*p0x - 2.0*t0x - t1x
	ax := t0x
	dx := p0x

	cy := 2.0*p0y - 2.0*p1y + t0y + t1y
	by := 3.0*p1y - 3.0*p0y - 2.0*t0y - t1y
	ay := t0y
	dy := p0y

	cz := 2.0*p0z - 2.0*p1z + t0z + t1z
	bz := 3.0*p1z - 3.0*p0z - 2.0*t0z - t1z
	az := t0z
	dz := p0z

	m.f3dTarget.SetTranslationX(tCubed*cx + tSquared*bx + t*ax + dx)
	m.f3dTarget.SetTranslationY(tCubed*cy + tSquared*by + t*ay + dy)
	m.f3dTarget.SetTranslationZ(tCubed*cz + tSquared*bz + t*az + dz)
}

func (m *EmonSpriteMotion) advanceF3DRoll(deltaSeconds float64) {
	m.resolveRoll()
	if len(m.f3dRoll) < 8 {
		return
	}

	if m.f3dTarget == nil {
		return
	}

	framesSinceLastCall := deltaSeconds * motionFramesPerSecond
	perFrameAngle := m.f3dRoll[7]
	if m.rollBudget > 0.0 {
		perFrameAngle = m.f3dRag / m.rollFrames
	}
	angleDelta := perFrameAngle * framesSinceLastCall
	if m.BaseKind() == 14 {
		angleDelta = -angleDelta
	}

	m.rollAngle += math.Abs(angleDelta)
	if angleDelta == 0.0 {
		return
	}

	m.applyF3DRollRotation(m.f3dTarget, m.f3dRoll, angleDelta)
}

func (m *EmonSpriteMotion) rollProgress(motionFrames float64) float64 {
	m.resolveRoll()
	if m.rollBudget > 0.0 {
		return math.Min(1.0, m.rollAngle/m.rollBudget)
	}

	return clampFloat(motionFrames/m.rollFrames, 0.0, 1.0)
}

func (m *EmonSpriteMotion) resolveRoll() {
	if m.rollResolved {
		return
	}

	m.rollResolved = true
	m.rollBudget = math.Abs(m.f3dRag)
	m.rollFrames = m.rollBudgetFrames()
}

func (m *EmonSpriteMotion) rollBudgetFrames() float64 {
	if m.duration > 0 {
		return float64(m.duration)
	}

	radius := 0.0
	if len(m.f3dRoll) >= 8 {
		radius = math.Abs(m.f3dRoll[7])
	}
	if !(radius > minimumRollDegrees) {
		return maximumRollFrames
	}

	return clampFloat(fullRollDegrees/radius, minimumRollFrames, maximumRollFrames)
}

func (m *EmonSpriteMotion) applyF3DRollRotation(target *EmonF3DState, roll []float64, angleDegrees float64) {
	p1x := roll[0]
	p1y := roll[1]
	p1z := roll[2]
	p2x := roll[3]
	p2y := roll[4]
	p2z := roll[5]

	axisX := p2x - p1x
	axisY := p2y - p1y
	axisZ := p2z - p1z

	axisLenSq := axisX*axisX + axisY*axisY + axisZ*axisZ
	if axisLenSq <= 0.0 {
		return
	}

	axisLen := math.Sqrt(axisLenSq)
	ux := axisX / axisLen
	uy := axisY / axisLen
	uz := axisZ / axisLen

	rad := angleDegrees * math.Pi / 180.0
	c := math.Cos(rad)
	s := math.Sin(rad)
	t := 1.0 - c

	r0 := t*ux*ux + c
	r1 := t*ux*uy + s*uz
	r2 := t*ux*uz - s*uy
	r4 := t*ux*uy - s*uz
	r5 := t*uy*uy + c
	r6 := t*uy*uz + s*ux
	r8 := t*ux*uz + s*uy
	r9 := t*uy*uz - s*ux
	r10 := t*uz*uz + c

	rp1x := r0*p1x + r4*p1y + r8*p1z
	rp1y := r1*p1x + r5*p1y + r9*p1z
	rp1z := r2*p1x + r6*p1y + r10*p1z

	scratch := m.f3dScratch[:]
	scratch[0] = r0
	scratch[1] = r1
	scratch[2] = r2
	scratch[3] = 0.0
	scratch[4] = r4
	scratch[5] = r5
	scratch[6] = r6
	scratch[7] = 0.0
	scratch[8] = r8
	scratch[9] = r9
	scratch[10] = r10
	scratch[11] = 0.0
	scratch[12] = p1x - rp1x
	scratch[13] = p1y - rp1y
	scratch[14] = p1z - rp1z
	scratch[15] = 1.0

	matrix := target.Matrix()
	F3DProjectorMultiply(scratch, matrix, scratch)
	copy(matrix, scratch)
	copy(target.Source(), matrix)
}
