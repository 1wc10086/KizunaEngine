package translator

func chunkIndexes(n int, size int) [][]int {
	if n <= 0 {
		return nil
	}
	if size <= 0 {
		size = 1
	}
	var chunks [][]int
	for start := 0; start < n; start += size {
		end := start + size
		if end > n {
			end = n
		}
		idx := make([]int, 0, end-start)
		for i := start; i < end; i++ {
			idx = append(idx, i)
		}
		chunks = append(chunks, idx)
	}
	return chunks
}
