package dotnet

import "time"

const maxRandomValue = int32(2147483647)

type Random struct {
	seedArray [56]int32
	inext     int
	inextp    int
}

func NewRandom() *Random {
	return NewRandomSeed(int32(time.Now().UnixNano() & 0x7fffffff))
}

func NewRandomSeed(seed int32) *Random {
	r := &Random{}
	subtraction := seed
	if subtraction == -2147483648 {
		subtraction = maxRandomValue
	} else if subtraction < 0 {
		subtraction = -subtraction
	}
	mj := int32(161803398) - subtraction
	r.seedArray[55] = mj
	mk := int32(1)
	for i := 1; i < 55; i++ {
		ii := (21 * i) % 55
		r.seedArray[ii] = mk
		mk = mj - mk
		if mk < 0 {
			mk += maxRandomValue
		}
		mj = r.seedArray[ii]
	}
	for k := 1; k < 5; k++ {
		for i := 1; i < 56; i++ {
			r.seedArray[i] -= r.seedArray[1+(i+30)%55]
			if r.seedArray[i] < 0 {
				r.seedArray[i] += maxRandomValue
			}
		}
	}
	r.inext = 0
	r.inextp = 21
	return r
}

func (r *Random) Sample() float64 {
	return float64(r.internalSample()) * (1.0 / 2147483647.0)
}

func (r *Random) NextDouble() float64 {
	return r.Sample()
}

func (r *Random) Next(maxValue int) int {
	if maxValue <= 0 {
		return 0
	}
	return int(r.Sample() * float64(maxValue))
}

func (r *Random) internalSample() int32 {
	locINext := r.inext
	locINextp := r.inextp
	locINext++
	if locINext >= 56 {
		locINext = 1
	}
	locINextp++
	if locINextp >= 56 {
		locINextp = 1
	}
	retVal := r.seedArray[locINext] - r.seedArray[locINextp]
	if retVal == maxRandomValue {
		retVal--
	}
	if retVal < 0 {
		retVal += maxRandomValue
	}
	r.seedArray[locINext] = retVal
	r.inext = locINext
	r.inextp = locINextp
	return retVal
}
