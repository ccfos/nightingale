/*
This package reads and writes pickled data. The format is the same
as the Python "pickle" module.

Protocols 0,1,2 are implemented. These are the versions written by the Python
2.x series. Python 3 defines newer protocol versions, but can write the older
protocol versions so they are readable by this package.

To read data, see stalecucumber.Unpickle.

To write data, see stalecucumber.NewPickler.

TLDR

Read a pickled string or unicode object
	pickle.dumps("foobar")
	---
	var somePickledData io.Reader
	mystring, err := stalecucumber.String(stalecucumber.Unpickle(somePickledData))

Read a pickled integer
	pickle.dumps(42)
	---
	var somePickledData io.Reader
	myint64, err := stalecucumber.Int(stalecucumber.Unpickle(somePickledData))

Read a pickled list of numbers into a structure
	pickle.dumps([8,8,2005])
	---
	var somePickledData io.Reader
	numbers := make([]int64,0)

	err := stalecucumber.UnpackInto(&numbers).From(stalecucumber.Unpickle(somePickledData))

Read a pickled dictionary into a structure
	pickle.dumps({"apple":1,"banana":2,"cat":"hello","Dog":42.0})
	---
	var somePickledData io.Reader
	mystruct := struct{
		Apple int
		Banana uint
		Cat string
		Dog float32}{}

	err := stalecucumber.UnpackInto(&mystruct).From(stalecucumber.Unpickle(somePickledData))

Pickle a struct

	buf := new(bytes.Buffer)
	mystruct := struct{
			Apple int
			Banana uint
			Cat string
			Dog float32}{}

	err := stalecucumber.NewPickler(buf).Pickle(mystruct)



Recursive objects

You can pickle recursive objects like so

	a = {}
	a["self"] = a
	pickle.dumps(a)

Python's pickler is intelligent enough not to emit an infinite data structure
when a recursive object is pickled.

I recommend against pickling recursive objects in the first place, but this
library handles unpickling them without a problem. The result of unpickling
the above is map[interface{}]interface{} with a key "a" that contains
a reference to itself.

Attempting to unpack the result of the above python code into a structure
with UnpackInto would either fail or recurse forever.

Protocol Performance

If the version of Python you are using supports protocol version 1 or 2,
you should always specify that protocol version. By default the "pickle"
and "cPickle" modules in Python write using protocol 0. Protocol 0
requires much more space to represent the same values and is much
slower to parse.

Unsupported Opcodes

The pickle format is incredibly flexible and as a result has some
features that are impractical or unimportant when implementing a reader in
another language.

Each set of opcodes is listed below by protocol version with the impact.

Protocol 0

	GLOBAL

This opcode is equivalent to calling "import foo; foo.bar" in python. It is
generated whenever an object instance, class definition, or method definition
is serialized. As long as the pickled data does not contain an instance
of a python class or a reference to a python callable this opcode is not
emitted by the "pickle" module.

A few examples of what will definitely cause this opcode to be emitted

	pickle.dumps(range) #Pickling the range function
	pickle.dumps(Exception()) #Pickling an instance of a python class

This opcode will be partially supported in a future revision to this package
that allows the unpickling of instances of Python classes.

	REDUCE
	BUILD
	INST

These opcodes are used in recreating pickled python objects. That is currently
not supported by this package.

These opcodes will be supported in a future revision to this package
that allows the unpickling of instances of Python classes.

	PERSID

This opcode is used to reference concrete definitions of objects between
a pickler and an unpickler by an ID number. The pickle protocol doesn't define
what a persistent ID means.

This opcode is unlikely to ever be supported by this package.

Protocol 1

	OBJ

This opcodes is used in recreating pickled python objects. That is currently
not supported by this package.

This opcode will supported in a future revision to this package
that allows the unpickling of instances of Python classes.


	BINPERSID

This opcode is equivalent to PERSID in protocol 0 and won't be supported
for the same reason.

Protocol 2

	NEWOBJ

This opcodes is used in recreating pickled python objects. That is currently
not supported by this package.

This opcode will supported in a future revision to this package
that allows the unpickling of instances of Python classes.

	EXT1
	EXT2
	EXT4

These opcodes allow using a registry
of popular objects that are pickled by name, typically classes.
It is envisioned that through a global negotiation and
registration process, third parties can set up a mapping between
ints and object names.

These opcodes are unlikely to ever be supported by this package.

*/
package stalecucumber

import "errors"
import "io"
import "bytes"
import "encoding/binary"
import "fmt"

var ErrOpcodeStopped = errors.New("STOP opcode found")
var ErrStackTooSmall = errors.New("Stack is too small to perform requested operation")
var ErrInputTruncated = errors.New("Input to the pickle machine was truncated")
var ErrOpcodeNotImplemented = errors.New("Input encountered opcode that is not implemented")
var ErrNoResult = errors.New("Input did not place a value onto the stack")
var ErrMarkNotFound = errors.New("Mark could not be found on the stack")

/*
Unpickle a value from a reader. This function takes a reader and
attempts to read a complete pickle program from it. This is normally
the output of the function "pickle.dump" from Python.

The returned type is interface{} because unpickling can generate any type. Use
a helper function to convert to another type without an additional type check.

This function returns an error if
the reader fails, the pickled data is invalid, or if the pickled data contains
an unsupported opcode. See unsupported opcodes in the documentation of
this package for more information.

Type Conversions

Types conversion Python types to Go types is performed as followed
	int -> int64
	string -> string
	unicode -> string
	float -> float64
	long -> big.Int from the "math/big" package
	lists -> []interface{}
	tuples -> []interface{}
	dict -> map[interface{}]interface{}

The following values are converted from Python to the Go types
	True & False -> bool
	None -> stalecucumber.PickleNone, sets pointers to nil

Helper Functions

The following helper functions were inspired by the github.com/garyburd/redigo
package. Each function takes the result of Unpickle as its arguments. If unpickle
fails it does nothing and returns that error. Otherwise it attempts to
convert to the appropriate type. If type conversion fails it returns an error

	String - string from Python string or unicode
	Int - int64 from Python int or long
	Bool - bool from Python True or False
	Big - *big.Int from Python long
	ListOrTuple - []interface{} from Python Tuple or List
	Float - float64 from Python float
	Dict - map[interface{}]interface{} from Python dictionary
	DictString -
		map[string]interface{} from Python dictionary.
		Keys must all be of type unicode or string.

Unpacking into structures

If the pickled object is a python dictionary that has only unicode and string
objects for keys, that object can be unpickled into a struct in Go by using
the "UnpackInto" function. The "From" receiver on the return value accepts
the result of "Unpickle" as its actual parameters.

The keys of the python dictionary are assigned to fields in a structure.
Structures may specify the tag "pickle" on fields. The value of this tag is taken
as the key name of the Python dictionary value to place in this field. If no
field has a matching "pickle" tag the fields are looked up by name. If
the first character of the key is not uppercase, it is uppercased. If a field
matching that name is found, the value in the python dictionary is unpacked
into the value of the field within the structure.

A list of python dictionaries can be unpickled into a slice of structures in
Go.

A homogeneous list of python values can be unpickled into a slice in
Go with the appropriate element type.

A nested python dictionary is unpickled into nested structures in Go. If a
field is of type map[interface{}]interface{} it is of course unpacked into that
as well.

By default UnpackInto skips any missing fields and fails if a field's
type is not compatible with the object's type.

This behavior can be changed by setting "AllowMissingFields" and
"AllowMismatchedFields" on the return value of UnpackInto before calling
From.

*/
func Unpickle(reader io.Reader) (interface{}, error) {
	var pm PickleMachine
	pm.buf = &bytes.Buffer{}
	pm.Reader = reader
	pm.lastMark = -1
	//Pre allocate a small stack
	pm.Stack = make([]interface{}, 0, 16)

	err := (&pm).execute()
	if err != ErrOpcodeStopped {
		return nil, pm.error(err)
	}

	if len(pm.Stack) == 0 {
		return nil, ErrNoResult
	}

	return pm.Stack[0], nil

}

var jumpList = buildEmptyJumpList()

func init() {
	populateJumpList(&jumpList)
}

/*
This type is returned whenever Unpickle encounters an error in pickled data.
*/
type PickleMachineError struct {
	Err       error
	StackSize int
	MemoSize  int
	Opcode    uint8
}

/*
This struct is current exposed but not useful. It is likely to be hidden
in the near future.
*/
type PickleMachine struct {
	Stack  []interface{}
	Memo   []interface{}
	Reader io.Reader

	currentOpcode uint8
	buf           *bytes.Buffer
	lastMark      int

	memoBuffer               [16]memoBufferElement
	memoBufferMaxDestination int64
	memoBufferIndex          int
}

type memoBufferElement struct {
	Destination int64
	V           interface{}
}

func (pme PickleMachineError) Error() string {
	return fmt.Sprintf("Pickle Machine failed on opcode:0x%x. Stack size:%d. Memo size:%d. Cause:%v", pme.Opcode, pme.StackSize, pme.MemoSize, pme.Err)
}

func (pm *PickleMachine) error(src error) error {
	return PickleMachineError{
		StackSize: len(pm.Stack),
		MemoSize:  len(pm.Memo),
		Err:       src,
		Opcode:    pm.currentOpcode,
	}
}

func (pm *PickleMachine) execute() error {
	for {
		err := binary.Read(pm.Reader, binary.BigEndian, &pm.currentOpcode)
		if err != nil {
			return err
		}

		err = jumpList[int(pm.currentOpcode)](pm)

		if err != nil {
			return err
		}
	}
}

func (pm *PickleMachine) flushMemoBuffer(vIndex int64, v interface{}) {
	//Extend the memo until it is large enough
	if pm.memoBufferMaxDestination >= int64(len(pm.Memo)) {
		replacement := make([]interface{}, pm.memoBufferMaxDestination<<1)
		copy(replacement, pm.Memo)
		pm.Memo = replacement
	}

	//If a value was passed into this function, write it into the memo
	//as well
	if vIndex != -1 {
		pm.Memo[vIndex] = v
	}

	//Write the contents of the buffer into the memo
	//in the same order as the puts were issued
	for i := 0; i != pm.memoBufferIndex; i++ {
		buffered := pm.memoBuffer[i]
		pm.Memo[buffered.Destination] = buffered.V
	}

	//Reset the buffer
	pm.memoBufferIndex = 0
	pm.memoBufferMaxDestination = 0

	return
}

func (pm *PickleMachine) storeMemo(index int64, v interface{}) error {
	if index < 0 {
		return fmt.Errorf("Requested to write to invalid memo index:%v", index)
	}

	//If there is space in the memo presently, then store it
	//and it is done.
	if index < int64(len(pm.Memo)) {
		pm.Memo[index] = v
		return nil
	}

	//Update the maximum index in the buffer if need be
	if index > pm.memoBufferMaxDestination {
		pm.memoBufferMaxDestination = index
	}

	//If the buffer is not full write into it
	if pm.memoBufferIndex != len(pm.memoBuffer) {
		pm.memoBuffer[pm.memoBufferIndex].V = v
		pm.memoBuffer[pm.memoBufferIndex].Destination = index
		pm.memoBufferIndex++
	} else {
		//If the buffer is full flush it now
		pm.flushMemoBuffer(index, v)
	}

	return nil
}

func (pm *PickleMachine) readFromMemo(index int64) (interface{}, error) {
	if index < 0 {
		return nil, fmt.Errorf("Requested to read from negative memo index %d", index)

	}

	//Test to see if the value is outside the current length of the memo
	if index >= int64(len(pm.Memo)) {
		pm.flushMemoBuffer(-1, nil)
		if index >= int64(len(pm.Memo)) {
			return nil, fmt.Errorf("Requested to read from invalid memo index %d", index)
		}
	}

	//Grab the value
	retval := pm.Memo[index]

	//If nil then flush the memo buffer to see if it is within it
	if retval == nil {
		pm.flushMemoBuffer(-1, nil)
		//Grab the value again after the flush
		retval = pm.Memo[index]
		//If still nil, then this is a read from an invalid position
		if retval == nil {
			return nil, fmt.Errorf("Requested to read from invalid memo index %d", index)
		}
	}

	return retval, nil
}

func (pm *PickleMachine) push(v interface{}) {
	pm.Stack = append(pm.Stack, v)
}

func (pm *PickleMachine) pop() (interface{}, error) {
	l := len(pm.Stack)
	if l == 0 {
		return nil, ErrStackTooSmall
	}

	l--
	top := pm.Stack[l]
	pm.Stack = pm.Stack[:l]
	return top, nil
}

func (pm *PickleMachine) readFromStack(offset int) (interface{}, error) {
	return pm.readFromStackAt(len(pm.Stack) - 1 - offset)
}

func (pm *PickleMachine) readFromStackAt(position int) (interface{}, error) {

	if position < 0 {
		return nil, fmt.Errorf("Request to read from invalid stack position %d", position)
	}

	return pm.Stack[position], nil

}

func (pm *PickleMachine) readIntFromStack(offset int) (int64, error) {
	v, err := pm.readFromStack(offset)
	if err != nil {
		return 0, err
	}

	vi, ok := v.(int64)
	if !ok {
		return 0, fmt.Errorf("Type %T was requested from stack but found %v(%T)", vi, v, v)
	}

	return vi, nil
}

func (pm *PickleMachine) popAfterIndex(index int) {
	//Input to this function must be sane, no checking is done

	/**
	if len(pm.Stack)-1 < index {
		return ErrStackTooSmall
	}**/

	pm.Stack = pm.Stack[0:index]
}

func (pm *PickleMachine) findMark() (int, error) {
	if pm.lastMark != -1 {
		mark := pm.lastMark
		pm.lastMark = -1
		if mark < len(pm.Stack) {
			if _, ok := pm.Stack[mark].(PickleMark); ok {
				return mark, nil
			}
		}
	}

	for i := len(pm.Stack) - 1; i != -1; i-- {
		if _, ok := pm.Stack[i].(PickleMark); ok {
			return i, nil
		}
	}
	return -1, ErrMarkNotFound
}

func (pm *PickleMachine) readFixedLengthRaw(l int64) ([]byte, error) {

	pm.buf.Reset()
	_, err := io.CopyN(pm.buf, pm.Reader, l)
	if err != nil {
		return nil, err
	}

	return pm.buf.Bytes(), nil
}

func (pm *PickleMachine) readFixedLengthString(l int64) (string, error) {

	//Avoid getting "<nil>"
	if l == 0 {
		return "", nil
	}

	pm.buf.Reset()
	_, err := io.CopyN(pm.buf, pm.Reader, l)
	if err != nil {
		return "", err
	}
	return pm.buf.String(), nil
}

func (pm *PickleMachine) readBytes() ([]byte, error) {
	//This is slow and protocol 0 only
	pm.buf.Reset()
	for {
		var v [1]byte
		n, err := pm.Reader.Read(v[:])
		if n != 1 {
			return nil, ErrInputTruncated
		}
		if err != nil {
			return nil, err
		}

		if v[0] == '\n' {
			break
		}
		pm.buf.WriteByte(v[0])
	}

	return pm.buf.Bytes(), nil
}

func (pm *PickleMachine) readString() (string, error) {
	//This is slow and protocol 0 only
	pm.buf.Reset()
	for {
		var v [1]byte
		n, err := pm.Reader.Read(v[:])
		if n != 1 {
			return "", ErrInputTruncated
		}
		if err != nil {
			return "", err
		}

		if v[0] == '\n' {
			break
		}
		pm.buf.WriteByte(v[0])
	}

	//Avoid getting "<nil>"
	if pm.buf.Len() == 0 {
		return "", nil
	}
	return pm.buf.String(), nil
}

func (pm *PickleMachine) readBinaryInto(dst interface{}, bigEndian bool) error {
	var bo binary.ByteOrder
	if bigEndian {
		bo = binary.BigEndian
	} else {
		bo = binary.LittleEndian
	}
	return binary.Read(pm.Reader, bo, dst)
}
