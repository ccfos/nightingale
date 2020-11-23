package stalecucumber

/*
This type is used internally to represent a concept known as a mark
on the Pickle Machine's stack. Oddly formed pickled data could return
this value as the result of Unpickle. In normal usage this type
is needed only internally.
*/
type PickleMark struct{}

func (_ PickleMark) String() string {
	return "PickleMachine Mark"
}

/*
This type is used to represent the Python object "None"
*/
type PickleNone struct{}

func (_ PickleNone) String() string {
	return "Python None"
}
