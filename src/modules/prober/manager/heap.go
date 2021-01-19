package manager

type ruleSummary struct {
	id       int64 // collect rule id
	activeAt int64
}

type ruleSummaryHeap []*ruleSummary

func (h ruleSummaryHeap) Len() int {
	return len(h)
}

func (h ruleSummaryHeap) Less(i, j int) bool {
	return h[i].activeAt < h[j].activeAt
}

func (h ruleSummaryHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *ruleSummaryHeap) Push(x interface{}) {
	*h = append(*h, x.(*ruleSummary))
}

func (h *ruleSummaryHeap) Pop() interface{} {
	x := (*h)[len(*h)-1]
	*h = (*h)[:len(*h)-1]
	return x
}

func (h *ruleSummaryHeap) Top() *ruleSummary {
	return (*h)[0]
}
