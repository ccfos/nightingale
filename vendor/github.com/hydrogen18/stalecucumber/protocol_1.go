package stalecucumber

import "fmt"
import "errors"

/**
Opcode: BININT (0x4a)
Push a four-byte signed integer.

      This handles the full range of Python (short) integers on a 32-bit
      box, directly as binary bytes (1 for the opcode and 4 for the integer).
      If the integer is non-negative and fits in 1 or 2 bytes, pickling via
      BININT1 or BININT2 saves space.
      **
Stack before: []
Stack after: [int]
**/
func (pm *PickleMachine) opcode_BININT() error {
	var v int32
	err := pm.readBinaryInto(&v, false)
	if err != nil {
		return err
	}

	pm.push(int64(v))
	return nil
}

/**
Opcode: BININT1 (0x4b)
Push a one-byte unsigned integer.

      This is a space optimization for pickling very small non-negative ints,
      in range(256).
      **
Stack before: []
Stack after: [int]
**/
func (pm *PickleMachine) opcode_BININT1() error {
	var v uint8
	err := pm.readBinaryInto(&v, false)
	if err != nil {
		return err
	}
	pm.push(int64(v))
	return nil
}

/**
Opcode: BININT2 (0x4d)
Push a two-byte unsigned integer.

      This is a space optimization for pickling small positive ints, in
      range(256, 2**16).  Integers in range(256) can also be pickled via
      BININT2, but BININT1 instead saves a byte.
      **
Stack before: []
Stack after: [int]
**/
func (pm *PickleMachine) opcode_BININT2() error {
	var v uint16
	err := pm.readBinaryInto(&v, false)
	if err != nil {
		return err
	}
	pm.push(int64(v))
	return nil

}

/**
Opcode: BINSTRING (0x54)
Push a Python string object.

      There are two arguments:  the first is a 4-byte little-endian signed int
      giving the number of bytes in the string, and the second is that many
      bytes, which are taken literally as the string content.
      **
Stack before: []
Stack after: [str]
**/
func (pm *PickleMachine) opcode_BINSTRING() error {
	var strlen int32
	err := pm.readBinaryInto(&strlen, false)
	if err != nil {
		return err
	}

	if strlen < 0 {
		return fmt.Errorf("BINSTRING specified negative string length of %d", strlen)
	}

	str, err := pm.readFixedLengthString(int64(strlen))
	if err != nil {
		return err
	}
	pm.push(str)
	return nil
}

/**
Opcode: SHORT_BINSTRING (0x55)
Push a Python string object.

      There are two arguments:  the first is a 1-byte unsigned int giving
      the number of bytes in the string, and the second is that many bytes,
      which are taken literally as the string content.
      **
Stack before: []
Stack after: [str]
**/
func (pm *PickleMachine) opcode_SHORT_BINSTRING() error {
	var strlen uint8
	err := pm.readBinaryInto(&strlen, false)
	if err != nil {
		return err
	}

	if strlen < 0 {
		return fmt.Errorf("SHORT_BINSTRING specified negative string length of %d", strlen)
	}

	str, err := pm.readFixedLengthString(int64(strlen))
	if err != nil {
		return err
	}
	pm.push(str)
	return nil
}

/**
Opcode: BINUNICODE (0x58)
Push a Python Unicode string object.

      There are two arguments:  the first is a 4-byte little-endian signed int
      giving the number of bytes in the string.  The second is that many
      bytes, and is the UTF-8 encoding of the Unicode string.
      **
Stack before: []
Stack after: [unicode]
**/
func (pm *PickleMachine) opcode_BINUNICODE() error {
	var l int32
	err := pm.readBinaryInto(&l, false)
	if err != nil {
		return err
	}

	str, err := pm.readFixedLengthString(int64(l))
	if err != nil {
		return err
	}

	pm.push(str)
	return nil
}

/**
Opcode: BINFLOAT (0x47)
Float stored in binary form, with 8 bytes of data.

      This generally requires less than half the space of FLOAT encoding.
      In general, BINFLOAT cannot be used to transport infinities, NaNs, or
      minus zero, raises an exception if the exponent exceeds the range of
      an IEEE-754 double, and retains no more than 53 bits of precision (if
      there are more than that, "add a half and chop" rounding is used to
      cut it back to 53 significant bits).
      **
Stack before: []
Stack after: [float]
**/
func (pm *PickleMachine) opcode_BINFLOAT() error {
	var v float64
	err := pm.readBinaryInto(&v, true)
	if err != nil {
		return err
	}

	pm.push(v)
	return nil

}

/**
Opcode: EMPTY_LIST (0x5d)
Push an empty list.**
Stack before: []
Stack after: [list]
**/
func (pm *PickleMachine) opcode_EMPTY_LIST() error {
	v := make([]interface{}, 0)
	pm.push(v)
	return nil
}

/**
Opcode: APPENDS (0x65)
Extend a list by a slice of stack objects.

      Stack before:  ... pylist markobject stackslice
      Stack after:   ... pylist+stackslice

      although pylist is really extended in-place.
      **
Stack before: [list, mark, stackslice]
Stack after: [list]
**/
func (pm *PickleMachine) opcode_APPENDS() error {
	markIndex, err := pm.findMark()
	if err != nil {
		return err
	}

	pyListI, err := pm.readFromStackAt(markIndex - 1)
	if err != nil {
		return err
	}

	pyList, ok := pyListI.([]interface{})
	if !ok {
		return fmt.Errorf("APPENDS expected type %T but got (%v)%T", pyList, pyListI, pyListI)
	}

	pyList = append(pyList, pm.Stack[markIndex+1:]...)
	pm.popAfterIndex(markIndex - 1)

	/**
	if err != nil {
		return err
	}**/

	pm.push(pyList)
	return nil
}

/**
Opcode: EMPTY_TUPLE (0x29)
Push an empty tuple.**
Stack before: []
Stack after: [tuple]
**/
func (pm *PickleMachine) opcode_EMPTY_TUPLE() error {
	return pm.opcode_EMPTY_LIST()
}

/**
Opcode: EMPTY_DICT (0x7d)
Push an empty dict.**
Stack before: []
Stack after: [dict]
**/
func (pm *PickleMachine) opcode_EMPTY_DICT() error {
	pm.push(make(map[interface{}]interface{}))
	return nil
}

/**
Opcode: SETITEMS (0x75)
Add an arbitrary number of key+value pairs to an existing dict.

      The slice of the stack following the topmost markobject is taken as
      an alternating sequence of keys and values, added to the dict
      immediately under the topmost markobject.  Everything at and after the
      topmost markobject is popped, leaving the mutated dict at the top
      of the stack.

      Stack before:  ... pydict markobject key_1 value_1 ... key_n value_n
      Stack after:   ... pydict

      where pydict has been modified via pydict[key_i] = value_i for i in
      1, 2, ..., n, and in that order.
      **
Stack before: [dict, mark, stackslice]
Stack after: [dict]
**/
func (pm *PickleMachine) opcode_SETITEMS() (err error) {
	defer func() {
		if r := recover(); r != nil {
			switch x := r.(type) {
			case string:
				err = errors.New(x)
			case error:
				err = x
			default:
				err = errors.New("Unknown panic")
			}
		}
	}()
	markIndex, err := pm.findMark()
	if err != nil {
		return err
	}

	vI, err := pm.readFromStackAt(markIndex - 1)
	if err != nil {
		return err
	}

	v, ok := vI.(map[interface{}]interface{})
	if !ok {
		return fmt.Errorf("Opcode SETITEMS expected type %T on stack but found %v(%T)", v, vI, vI)
	}

	if ((len(pm.Stack) - markIndex + 1) % 2) != 0 {
		return fmt.Errorf("Found odd number of items on stack after mark:%d", len(pm.Stack)-markIndex+1)
	}

	for i := markIndex + 1; i != len(pm.Stack); i++ {
		key := pm.Stack[i]
		i++
		v[key] = pm.Stack[i]
	}

	pm.popAfterIndex(markIndex)

	return nil
}

/**
Opcode: POP_MARK (0x31)
Pop all the stack objects at and above the topmost markobject.

      When an opcode using a variable number of stack objects is done,
      POP_MARK is used to remove those objects, and to remove the markobject
      that delimited their starting position on the stack.
      **
Stack before: [mark, stackslice]
Stack after: []
**/
func (pm *PickleMachine) opcode_POP_MARK() error {
	markIndex, err := pm.findMark()
	if err != nil {
		return nil
	}
	pm.popAfterIndex(markIndex)
	return nil
}

/**
Opcode: BINGET (0x68)
Read an object from the memo and push it on the stack.

      The index of the memo object to push is given by the 1-byte unsigned
      integer following.
      **
Stack before: []
Stack after: [any]
**/
func (pm *PickleMachine) opcode_BINGET() (err error) {
	defer func() {
		if r := recover(); r != nil {
			switch x := r.(type) {
			case string:
				err = errors.New(x)
			case error:
				err = x
			default:
				err = errors.New("Unknown panic")
			}
		}
	}()
	var index uint8
	err = pm.readBinaryInto(&index, false)
	if err != nil {
		return err
	}

	v, err := pm.readFromMemo(int64(index))
	if err != nil {
		return err
	}

	//TODO test if the object we are about to push is mutable
	//if so it needs to be somehow deep copied first
	pm.push(v)

	return nil
}

/**
Opcode: LONG_BINGET (0x6a)
Read an object from the memo and push it on the stack.

      The index of the memo object to push is given by the 4-byte signed
      little-endian integer following.
      **
Stack before: []
Stack after: [any]
**/
func (pm *PickleMachine) opcode_LONG_BINGET() (err error) {
	defer func() {
		if r := recover(); r != nil {
			switch x := r.(type) {
			case string:
				err = errors.New(x)
			case error:
				err = x
			default:
				err = errors.New("Unknown panic")
			}
		}
	}()
	var index int32
	err = pm.readBinaryInto(&index, false)
	if err != nil {
		return err
	}

	v, err := pm.readFromMemo(int64(index))
	if err != nil {
		return err
	}

	//TODO test if the object we are about to push is mutable
	//if so it needs to be somehow deep copied first
	pm.push(v)
	return nil
}

/**
Opcode: BINPUT (0x71)
Store the stack top into the memo.  The stack is not popped.

      The index of the memo location to write into is given by the 1-byte
      unsigned integer following.
      **
Stack before: []
Stack after: []
**/
func (pm *PickleMachine) opcode_BINPUT() error {
	v, err := pm.readFromStack(0)
	if err != nil {
		return err
	}

	var index uint8
	err = pm.readBinaryInto(&index, false)
	if err != nil {
		return err
	}

	pm.storeMemo(int64(index), v)
	return nil
}

/**
Opcode: LONG_BINPUT (0x72)
Store the stack top into the memo.  The stack is not popped.

      The index of the memo location to write into is given by the 4-byte
      signed little-endian integer following.
      **
Stack before: []
Stack after: []
**/
func (pm *PickleMachine) opcode_LONG_BINPUT() error {
	var index int32
	err := pm.readBinaryInto(&index, false)
	if err != nil {
		return err
	}

	v, err := pm.readFromStack(0)
	if err != nil {
		return err
	}
	err = pm.storeMemo(int64(index), v)
	if err != nil {
		return err
	}
	return nil
}

/**
Opcode: OBJ (0x6f)
Build a class instance.

      This is the protocol 1 version of protocol 0's INST opcode, and is
      very much like it.  The major difference is that the class object
      is taken off the stack, allowing it to be retrieved from the memo
      repeatedly if several instances of the same class are created.  This
      can be much more efficient (in both time and space) than repeatedly
      embedding the module and class names in INST opcodes.

      Unlike INST, OBJ takes no arguments from the opcode stream.  Instead
      the class object is taken off the stack, immediately above the
      topmost markobject:

      Stack before: ... markobject classobject stackslice
      Stack after:  ... new_instance_object

      As for INST, the remainder of the stack above the markobject is
      gathered into an argument tuple, and then the logic seems identical,
      except that no __safe_for_unpickling__ check is done (XXX this is
      a bug; cPickle does test __safe_for_unpickling__).  See INST for
      the gory details.

      NOTE:  In Python 2.3, INST and OBJ are identical except for how they
      get the class object.  That was always the intent; the implementations
      had diverged for accidental reasons.
      **
Stack before: [mark, any, stackslice]
Stack after: [any]
**/
func (pm *PickleMachine) opcode_OBJ() error {
	return ErrOpcodeNotImplemented
}

/**
Opcode: BINPERSID (0x51)
Push an object identified by a persistent ID.

      Like PERSID, except the persistent ID is popped off the stack (instead
      of being a string embedded in the opcode bytestream).  The persistent
      ID is passed to self.persistent_load(), and whatever object that
      returns is pushed on the stack.  See PERSID for more detail.
      **
Stack before: [any]
Stack after: [any]
**/
func (pm *PickleMachine) opcode_BINPERSID() error {
	return ErrOpcodeNotImplemented
}
