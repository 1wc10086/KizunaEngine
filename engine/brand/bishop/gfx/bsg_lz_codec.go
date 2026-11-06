package gfx

import "kizuna/engine/core/binutil"

const bsgLzHeaderSize = 5

func UnpackBsgLz(source []byte, output []byte, plane, pixelSize int) int {
	if len(source) < bsgLzHeaderSize || len(output) <= plane {
		return 0
	}

	control := source[0]
	limit := int(binutil.I32(source[1:]))
	if limit > len(source) || limit < bsgLzHeaderSize {
		limit = len(source)
	}

	position := bsgLzHeaderSize
	destination := plane

	for position < limit {
		value := source[position]
		position++

		if value == control {
			if position >= limit {
				break
			}

			offset := source[position]
			position++
			if offset != control {
				if position >= limit {
					break
				}

				count := source[position]
				position++
				distance := int(offset)
				if offset > control {
					distance = int(offset) - 1
				}
				distance *= pixelSize

				for i := 0; i < int(count); i++ {
					if uint(destination) < uint(len(output)) {
						origin := destination - distance
						if uint(origin) < uint(len(output)) {
							output[destination] = output[origin]
						} else {
							output[destination] = 0
						}
					}

					destination += pixelSize
				}

				continue
			}
		}

		if uint(destination) < uint(len(output)) {
			output[destination] = value
		}

		destination += pixelSize
	}

	for i := plane + pixelSize; i < len(output); i += pixelSize {
		output[i] += output[i-pixelSize]
	}

	return min(limit, len(source))
}
