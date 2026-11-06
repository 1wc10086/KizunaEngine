package stage

import "math"

const EmonF3DIdentity = 0

type EmonF3DBounds struct {
	Left   int
	Top    int
	Right  int
	Bottom int
}

var EmonF3DBoundsEmpty = EmonF3DBounds{
	Left:   math.MaxInt32,
	Top:    math.MaxInt32,
	Right:  math.MinInt32,
	Bottom: math.MinInt32,
}

func (b EmonF3DBounds) HasValue() bool {
	return b.Left <= b.Right && b.Top <= b.Bottom
}

type EmonF3DSnapshot struct {
	Matrix  [16]float64
	Source  [16]float64
	CameraX float64
	CameraY float64
	CameraZ float64
}

type EmonF3DState struct {
	matrix  [16]float64
	source  [16]float64
	cameraX float64
	cameraY float64
	cameraZ float64
	bounds  EmonF3DBounds
}

func NewEmonF3DState() *EmonF3DState {
	state := &EmonF3DState{bounds: EmonF3DBoundsEmpty}
	setF3DIdentity(state.matrix[:])
	setF3DIdentity(state.source[:])
	return state
}

func (s *EmonF3DState) Matrix() []float64 {
	return s.matrix[:]
}

func (s *EmonF3DState) Source() []float64 {
	return s.source[:]
}

func (s *EmonF3DState) CameraX() float64 {
	return s.cameraX
}

func (s *EmonF3DState) CameraY() float64 {
	return s.cameraY
}

func (s *EmonF3DState) CameraZ() float64 {
	return s.cameraZ
}

func (s *EmonF3DState) Bounds() EmonF3DBounds {
	return s.bounds
}

func (s *EmonF3DState) TranslationX() float64 {
	return s.matrix[12]
}

func (s *EmonF3DState) SetTranslationX(value float64) {
	s.matrix[12] = value
}

func (s *EmonF3DState) TranslationY() float64 {
	return s.matrix[13]
}

func (s *EmonF3DState) SetTranslationY(value float64) {
	s.matrix[13] = value
}

func (s *EmonF3DState) TranslationZ() float64 {
	return s.matrix[14]
}

func (s *EmonF3DState) SetTranslationZ(value float64) {
	s.matrix[14] = value
}

func (s *EmonF3DState) SetCamera(x, y, z float64) {
	s.cameraX = x
	s.cameraY = y
	s.cameraZ = z
}

func (s *EmonF3DState) SetAngle(values []float64) {
	if len(values) < 9 {
		return
	}

	ax := values[0]
	ay := values[1]
	az := values[2]
	sx := values[3]
	sy := values[4]
	sz := values[5]
	tx := values[6]
	ty := values[7]
	tz := values[8]
	ux := 0.0
	uy := 0.0
	uz := 0.0
	if len(values) > 9 {
		ux = values[9]
	}
	if len(values) > 10 {
		uy = values[10]
	}
	if len(values) > 11 {
		uz = values[11]
	}

	rad := math.Pi / 180.0
	cphi := math.Cos(ax * rad)
	sphi := math.Sin(ax * rad)
	ctheta := math.Cos(ay * rad)
	stheta := math.Sin(ay * rad)
	cpsi := math.Cos(az * rad)
	spsi := math.Sin(az * rad)

	col0X := cpsi * ctheta * sx
	col0Y := cpsi*stheta*sphi - spsi*cphi
	col0Z := cpsi*stheta*cphi + spsi*sphi

	col1X := spsi * ctheta
	col1Y := (spsi*stheta*sphi + cpsi*cphi) * sy
	col1Z := spsi*stheta*cphi - cpsi*sphi

	col2X := -stheta
	col2Y := ctheta * sphi
	col2Z := ctheta * cphi * sz

	newTransX := (ux*s.matrix[0] + uy*s.matrix[4] + uz*s.matrix[8] + tx) * 0.98
	newTransY := (ux*s.matrix[1] + uy*s.matrix[5] + uz*s.matrix[9] + ty) * 0.98
	newTransZ := (ux*s.matrix[2] + uy*s.matrix[6] + uz*s.matrix[10] + tz) * 0.98

	s.source[0] = col0X
	s.source[1] = col0Y
	s.source[2] = col0Z
	s.source[3] = 0.0
	s.source[4] = col1X
	s.source[5] = col1Y
	s.source[6] = col1Z
	s.source[7] = 0.0
	s.source[8] = col2X
	s.source[9] = col2Y
	s.source[10] = col2Z
	s.source[11] = 0.0
	s.source[12] = newTransX
	s.source[13] = newTransY
	s.source[14] = newTransZ
	s.source[15] = 1.0

	copy(s.matrix[:], s.source[:])
}

func (s *EmonF3DState) SetScale(values []float64) {
	if len(values) < 6 {
		return
	}

	setF3DIdentity(s.source[:])
	s.source[0] = values[0]
	s.source[5] = values[1]
	s.source[10] = values[2]
	s.source[15] = 1.0

	s.source[12] = values[3]
	s.source[13] = values[4]
	s.source[14] = values[5]

	copy(s.matrix[:], s.source[:])
}

func (s *EmonF3DState) SetVector(values []float64) {
	if len(values) < 9 {
		return
	}

	setF3DIdentity(s.source[:])
	s.source[0] = values[0]
	s.source[1] = values[1]
	s.source[2] = values[2]
	s.source[4] = values[3]
	s.source[5] = values[4]
	s.source[6] = values[5]
	s.source[8] = values[6]
	s.source[9] = values[7]
	s.source[10] = values[8]
	s.source[15] = 1.0
	if len(values) > 11 {
		s.source[12] = values[9]
		s.source[13] = values[10]
		s.source[14] = values[11]
	}

	if len(values) > 14 {
		s.source[12] += values[12]
		s.source[13] += values[13]
		s.source[14] += values[14]
	}

	copy(s.matrix[:], s.source[:])
}

func (s *EmonF3DState) Reset() {
	setF3DIdentity(s.matrix[:])
	setF3DIdentity(s.source[:])
}

func (s *EmonF3DState) ToWorld(localX, localY float64) (float64, float64, float64) {
	return F3DProjectorTransform(s.matrix[:], localX, localY, 0)
}

func (s *EmonF3DState) Project(worldX, worldY, worldZ float64) (float64, float64, bool) {
	return F3DProjectorProjectToScreen(s.cameraX, s.cameraY, s.cameraZ, worldX, worldY, worldZ)
}

func (s *EmonF3DState) UpdateBounds(localX, localY, width, height float64) {
	minX := math.Inf(1)
	minY := math.Inf(1)
	maxX := math.Inf(-1)
	maxY := math.Inf(-1)

	corners := [4][2]float64{
		{localX, localY},
		{localX + width, localY},
		{localX, localY + height},
		{localX + width, localY + height},
	}

	for i := 0; i < len(corners); i++ {
		wx, wy, wz := s.ToWorld(corners[i][0], corners[i][1])
		sx, sy, ok := s.Project(wx, wy, wz)
		if !ok {
			s.bounds = EmonF3DBoundsEmpty
			return
		}

		if sx < minX {
			minX = sx
		}
		if sy < minY {
			minY = sy
		}
		if sx > maxX {
			maxX = sx
		}
		if sy > maxY {
			maxY = sy
		}
	}

	s.bounds = EmonF3DBounds{
		Left:   int(math.Floor(minX)),
		Top:    int(math.Floor(minY)),
		Right:  int(math.Ceil(maxX)),
		Bottom: int(math.Ceil(maxY)),
	}
}

func (s *EmonF3DState) Snapshot() EmonF3DSnapshot {
	return EmonF3DSnapshot{
		Matrix:  s.matrix,
		Source:  s.source,
		CameraX: s.cameraX,
		CameraY: s.cameraY,
		CameraZ: s.cameraZ,
	}
}

func (s *EmonF3DState) Restore(snapshot EmonF3DSnapshot) {
	s.matrix = snapshot.Matrix
	s.source = snapshot.Source
	s.cameraX = snapshot.CameraX
	s.cameraY = snapshot.CameraY
	s.cameraZ = snapshot.CameraZ
}

func setF3DIdentity(matrix []float64) {
	matrix[0] = 1.0
	matrix[1] = 0.0
	matrix[2] = 0.0
	matrix[3] = 0.0
	matrix[4] = 0.0
	matrix[5] = 1.0
	matrix[6] = 0.0
	matrix[7] = 0.0
	matrix[8] = 0.0
	matrix[9] = 0.0
	matrix[10] = 1.0
	matrix[11] = 0.0
	matrix[12] = 0.0
	matrix[13] = 0.0
	matrix[14] = 0.0
	matrix[15] = 1.0
}
