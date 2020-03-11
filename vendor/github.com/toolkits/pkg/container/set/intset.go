package set

type IntSet struct {
	M map[int]struct{}
}

func NewIntSet() *IntSet {
	return &IntSet{
		M: make(map[int]struct{}),
	}
}

func (this *IntSet) Add(elt int) *IntSet {
	this.M[elt] = struct{}{}
	return this
}

func (this *IntSet) Exists(elt int) bool {
	_, exists := this.M[elt]
	return exists
}

func (this *IntSet) Delete(elt int) {
	delete(this.M, elt)
}

func (this *IntSet) Clear() {
	this.M = make(map[int]struct{})
}

func (this *IntSet) ToSlice() []int {
	count := len(this.M)
	if count == 0 {
		return []int{}
	}

	r := make([]int, count)

	i := 0
	for elt := range this.M {
		r[i] = elt
		i++
	}

	return r
}
