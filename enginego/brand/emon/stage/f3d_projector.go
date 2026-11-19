package stage

func F3DProjectorMultiply(left, right, destination []float64) {
	var tmp [16]float64
	for c := 0; c < 4; c++ {
		for r := 0; r < 4; r++ {
			sum := 0.0
			for k := 0; k < 4; k++ {
				sum += left[k*4+r] * right[c*4+k]
			}
			tmp[c*4+r] = sum
		}
	}
	copy(destination, tmp[:])
}

func F3DProjectorTransform(matrix []float64, x, y, z float64) (float64, float64, float64) {
	nx := matrix[0]*x + matrix[4]*y + matrix[8]*z + matrix[12]
	ny := matrix[1]*x + matrix[5]*y + matrix[9]*z + matrix[13]
	nz := matrix[2]*x + matrix[6]*y + matrix[10]*z + matrix[14]
	return nx, ny, nz
}

func F3DProjectorProjectToScreen(cameraX, cameraY, cameraZ, worldX, worldY, worldZ float64) (float64, float64, bool) {
	dz := worldZ - cameraZ
	if dz == 0.0 {
		return 0.0, 0.0, false
	}
	scale := -cameraZ / dz
	return cameraX + scale*(worldX-cameraX), cameraY + scale*(worldY-cameraY), true
}

func F3DProjectorPerspectiveScale(cameraZ, worldZ float64) float64 {
	dz := worldZ - cameraZ
	if dz == 0.0 {
		return 0.0
	}
	return -cameraZ / dz
}
