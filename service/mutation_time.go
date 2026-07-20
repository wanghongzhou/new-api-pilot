package service

func monotonicMutationTime(now int64, previous int64) int64 {
	if now <= previous {
		return previous + 1
	}
	return now
}
