package stalecucumber

import "fmt"
import "math/big"

/**
Opcode: LONG1 (0x8a)
Long integer using one-byte length.
	An arbitrary length integer encoded as a bytestring.
	A single byte following the opcode indicates the length
	of the bytestring
	If the string length is zero, then the value is zero.
	Otherwise the bytestring is 256-complement representation
	of an integer in reverse.
      **
Stack before: []
Stack after: [long]
**/
func (pm *PickleMachine) opcode_LONG1() error {
	var l uint8
	err := pm.readBinaryInto(&l, false)
	if err != nil {
		return err
	}

	if l == 0 {
		pm.push(big.NewInt(0))
		return nil
	}

	reversedData, err := pm.readFixedLengthRaw(int64(l))
	if err != nil {
		return err
	}

	//For no obvious reason, the python pickler
	//always reverses the bytes. Reverse it here
	var data [256]byte
	{
		var j int
		for i := len(reversedData) - 1; i != -1; i-- {
			data[j] = reversedData[i]
			j++
		}
	}

	v := new(big.Int)
	v.SetBytes(data[:l])

	invertIfNegative(data[0], v, int(l))

	pm.push(v)

	return nil

}

func invertIfNegative(first byte, v *big.Int, l int) {
	var negative bool
	//Check for negative number.
	negative = (0x80 & first) != 0x0

	if negative {
		offset := big.NewInt(1)
		offset.Lsh(offset, uint(l*8))
		v.Sub(v, offset)
	}

}

/**
Opcode: LONG4 (0x8b)
Long integer using found-byte length.

    An arbitrary length integer encoded as a bytestring.
	A four-byte signed little-endian integer
	following the opcode indicates the length  of the bytestring.
	If the string length is zero, then the value is zero.
	Otherwise the bytestring is 256-complement representation
	of an integer in reverse.**
Stack before: []
Stack after: [long]
**/
func (pm *PickleMachine) opcode_LONG4() error {
	var l uint32
	err := pm.readBinaryInto(&l, false)
	if err != nil {
		return err
	}

	if l == 0 {
		pm.push(big.NewInt(0))
		return nil
	}

	reversedData, err := pm.readFixedLengthRaw(int64(l))
	if err != nil {
		return err
	}

	//For no obvious reason, the python pickler
	//always reverses the bytes. Reverse it here
	data := make([]byte, len(reversedData))
	{
		var j int
		for i := len(reversedData) - 1; i != -1; i-- {
			data[j] = reversedData[i]
			j++
		}
	}

	v := new(big.Int)
	v.SetBytes(data[:l])

	invertIfNegative(data[0], v, len(data))

	pm.push(v)

	return nil

}

/**
Opcode: NEWTRUE (0x88)
True.

      Push True onto the stack.**
Stack before: []
Stack after: [bool]
**/
func (pm *PickleMachine) opcode_NEWTRUE() error {
	pm.push(true)
	return nil
}

/**
Opcode: NEWFALSE (0x89)
True.

      Push False onto the stack.**
Stack before: []
Stack after: [bool]
**/
func (pm *PickleMachine) opcode_NEWFALSE() error {
	pm.push(false)
	return nil
}

/**
Opcode: TUPLE1 (0x85)
Build a one-tuple out of the topmost item on the stack.

      This code pops one value off the stack and pushes a tuple of
      length 1 whose one item is that value back onto it.  In other
      words:

          stack[-1] = tuple(stack[-1:])
      **
Stack before: [any]
Stack after: [tuple]
**/
func (pm *PickleMachine) opcode_TUPLE1() error {
	v, err := pm.pop()
	if err != nil {
		return err
	}

	pm.push([]interface{}{v})
	return nil
}

/**
Opcode: TUPLE2 (0x86)
Build a two-tuple out of the top two items on the stack.

      This code pops two values off the stack and pushes a tuple of
      length 2 whose items are those values back onto it.  In other
      words:

          stack[-2:] = [tuple(stack[-2:])]
      **
Stack before: [any, any]
Stack after: [tuple]
**/
func (pm *PickleMachine) opcode_TUPLE2() error {
	v := make([]interface{}, 2)
	var err error
	v[1], err = pm.pop()
	if err != nil {
		return err
	}
	v[0], err = pm.pop()
	if err != nil {
		return err
	}

	pm.push(v)
	return nil
}

/**
Opcode: TUPLE3 (0x87)
Build a three-tuple out of the top three items on the stack.

      This code pops three values off the stack and pushes a tuple of
      length 3 whose items are those values back onto it.  In other
      words:

          stack[-3:] = [tuple(stack[-3:])]
      **
Stack before: [any, any, any]
Stack after: [tuple]
**/
func (pm *PickleMachine) opcode_TUPLE3() error {
	v := make([]interface{}, 3)
	var err error
	v[2], err = pm.pop()
	if err != nil {
		return err
	}
	v[1], err = pm.pop()
	if err != nil {
		return err
	}
	v[0], err = pm.pop()
	if err != nil {
		return err
	}

	pm.push(v)
	return nil
}

/**
Opcode: EXT1 (0x82)
Extension code.

      This code and the similar EXT2 and EXT4 allow using a registry
      of popular objects that are pickled by name, typically classes.
      It is envisioned that through a global negotiation and
      registration process, third parties can set up a mapping between
      ints and object names.

      In order to guarantee pickle interchangeability, the extension
      code registry ought to be global, although a range of codes may
      be reserved for private use.

      EXT1 has a 1-byte integer argument.  This is used to index into the
      extension registry, and the object at that index is pushed on the stack.
      **
Stack before: []
Stack after: [any]
**/
func (pm *PickleMachine) opcode_EXT1() error {
	return ErrOpcodeNotImplemented
}

/**
Opcode: EXT2 (0x83)
Extension code.

      See EXT1.  EXT2 has a two-byte integer argument.
      **
Stack before: []
Stack after: [any]
**/
func (pm *PickleMachine) opcode_EXT2() error {
	return ErrOpcodeNotImplemented
}

/**
Opcode: EXT4 (0x84)
Extension code.

      See EXT1.  EXT4 has a four-byte integer argument.
      **
Stack before: []
Stack after: [any]
**/
func (pm *PickleMachine) opcode_EXT4() error {
	return ErrOpcodeNotImplemented
}

/**
Opcode: NEWOBJ (0x81)
Build an object instance.

      The stack before should be thought of as containing a class
      object followed by an argument tuple (the tuple being the stack
      top).  Call these cls and args.  They are popped off the stack,
      and the value returned by cls.__new__(cls, *args) is pushed back
      onto the stack.
      **
Stack before: [any, any]
Stack after: [any]
**/
func (pm *PickleMachine) opcode_NEWOBJ() error {
	return ErrOpcodeNotImplemented
}

/**
Opcode: PROTO (0x80)
Protocol version indicator.

      For protocol 2 and above, a pickle must start with this opcode.
      The argument is the protocol version, an int in range(2, 256).
      **
Stack before: []
Stack after: []
**/
func (pm *PickleMachine) opcode_PROTO() error {
	var version int8
	err := pm.readBinaryInto(&version, false)
	if err != nil {
		return err
	}
	if version != 2 {
		return fmt.Errorf("Unsupported version #%d detected", version)
	}

	return nil
}
