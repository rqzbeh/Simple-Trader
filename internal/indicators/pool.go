package indicators

import "sync"

const defaultPoolCap = 1024

var floatSlicePool = sync.Pool{
	New: func() interface{} {
		s := make([]float64, defaultPoolCap)
		return &s
	},
}

func getFloatSlice(n int) []float64 {
	if n > defaultPoolCap {
		return make([]float64, n)
	}
	sp := floatSlicePool.Get().(*[]float64)
	return (*sp)[:n]
}

func putFloatSlice(s []float64) {
	if cap(s) < defaultPoolCap {
		return
	}
	s = s[:defaultPoolCap]
	for i := range s {
		s[i] = 0
	}
	floatSlicePool.Put(&s)
}
