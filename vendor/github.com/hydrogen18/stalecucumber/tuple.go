package stalecucumber

type PickleTuple []interface{}

func NewTuple(v ...interface{}) PickleTuple {
	return PickleTuple(v)
}
