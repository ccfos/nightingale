package stalecucumber

import "strconv"
import "fmt"
import "math/big"
import "errors"

//import "unicode/utf8"
import "unicode/utf16"

/**
Opcode: INT
Push an integer or bool.

      The argument is a newline-terminated decimal literal string.

      The intent may have been that this always fit in a short Python int,
      but INT can be generated in pickles written on a 64-bit box that
      require a Python long on a 32-bit box.  The difference between this
      and LONG then is that INT skips a trailing 'L', and produces a short
      int whenever possible.

      Another difference is due to that, when bool was introduced as a
      distinct type in 2.3, builtin names True and False were also added to
      2.2.2, mapping to ints 1 and 0.  For compatibility in both directions,
      True gets pickled as INT + "I01\n", and False as INT + "I00\n".
      Leading zeroes are never produced for a genuine integer.  The 2.3
      (and later) unpicklers special-case these and return bool instead;
      earlier unpicklers ignore the leading "0" and return the int.
      **
Stack before: []
Stack after: [int_or_bool]
**/
func (pm *PickleMachine) opcode_INT() error {
	str, err := pm.readString()
	if err != nil {
		return err
	}

	//check for boolean sentinels
	if len(str) == 2 {
		switch str {
		case "01":
			pm.push(true)
			return nil
		case "00":
			pm.push(false)
			return nil
		default:
		}
	}

	n, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		return err
	}

	pm.push(n)
	return nil
}

/**
Opcode: LONG
Push a long integer.

      The same as INT, except that the literal ends with 'L', and always
      unpickles to a Python long.  There doesn't seem a real purpose to the
      trailing 'L'.

      Note that LONG takes time quadratic in the number of digits when
      unpickling (this is simply due to the nature of decimal->binary
      conversion).  Proto 2 added linear-time (in C; still quadratic-time
      in Python) LONG1 and LONG4 opcodes.
      **
Stack before: []
Stack after: [long]
**/
func (pm *PickleMachine) opcode_LONG() error {
	i := new(big.Int)
	str, err := pm.readString()
	if err != nil {
		return err
	}
	if len(str) == 0 {
		return fmt.Errorf("String for LONG opcode cannot be zero length")
	}

	last := str[len(str)-1]
	if last != 'L' {
		return fmt.Errorf("String for LONG opcode must end with %q not %q", "L", last)
	}
	v := str[:len(str)-1]
	_, err = fmt.Sscan(v, i)
	if err != nil {
		return err
	}
	pm.push(i)
	return nil
}

/**
Opcode: STRING
Push a Python string object.

      The argument is a repr-style string, with bracketing quote characters,
      and perhaps embedded escapes.  The argument extends until the next
      newline character.
      **
Stack before: []
Stack after: [str]
**/

var unquoteInputs = []byte{0x27, 0x22, 0x0}

func (pm *PickleMachine) opcode_STRING() error {
	str, err := pm.readString()
	if err != nil {
		return err
	}

	//For whatever reason, the string is quoted. So the first and last character
	//should always be the single quote
	if len(str) < 2 {
		return fmt.Errorf("For STRING opcode, argument has invalid length %d", len(str))
	}

	if str[0] != '\'' || str[len(str)-1] != '\'' {
		return fmt.Errorf("For STRING opcode, argument has poorly formed value %q", str)
	}

	v := str[1 : len(str)-1]

	f := make([]rune, 0, len(v))

	for len(v) != 0 {
		var vr rune
		var replacement string
		for _, i := range unquoteInputs {
			vr, _, replacement, err = strconv.UnquoteChar(v, i)
			if err == nil {
				break
			}
		}

		if err != nil {
			c := v[0]
			return fmt.Errorf("Read thus far %q. Failed to unquote character %c error:%v", string(f), c, err)
		}
		v = replacement

		f = append(f, vr)
	}

	pm.push(string(f))
	return nil
}

/**
Opcode: NONE
Push None on the stack.**
Stack before: []
Stack after: [None]
**/
func (pm *PickleMachine) opcode_NONE() error {
	pm.push(PickleNone{})
	return nil
}

/**
Opcode: UNICODE
Push a Python Unicode string object.

      The argument is a raw-unicode-escape encoding of a Unicode string,
      and so may contain embedded escape sequences.  The argument extends
      until the next newline character.
      **
Stack before: []
Stack after: [unicode]
**/
func (pm *PickleMachine) opcode_UNICODE() error {
	str, err := pm.readBytes()
	if err != nil {
		return err
	}

	f := make([]rune, 0, len(str))

	var total int
	var consumed int
	total = len(str)
	for total != consumed {
		h := str[consumed]

		//Python 'raw-unicode-escape' doesnt
		//escape extended ascii
		if h > 127 {
			ea := utf16.Decode([]uint16{uint16(h)})
			f = append(f, ea...)
			consumed += 1
			continue
		}

		//Multibyte unicode points are escaped
		//so use "UnquoteChar" to handle those
		var vr rune
		for _, i := range unquoteInputs {
			pre := string(str[consumed:])
			var post string
			vr, _, post, err = strconv.UnquoteChar(pre, i)
			if err == nil {
				consumed += len(pre) - len(post)
				break
			}

		}

		if err != nil {
			c := str[0]
			return fmt.Errorf("Read thus far %q. Failed to unquote character %c error:%v", string(f), c, err)
		}

		f = append(f, vr)
	}

	pm.push(string(f))

	return nil
}

/**
Opcode: FLOAT
Newline-terminated decimal float literal.

      The argument is repr(a_float), and in general requires 17 significant
      digits for roundtrip conversion to be an identity (this is so for
      IEEE-754 double precision values, which is what Python float maps to
      on most boxes).

      In general, FLOAT cannot be used to transport infinities, NaNs, or
      minus zero across boxes (or even on a single box, if the platform C
      library can't read the strings it produces for such things -- Windows
      is like that), but may do less damage than BINFLOAT on boxes with
      greater precision or dynamic range than IEEE-754 double.
      **
Stack before: []
Stack after: [float]
**/
func (pm *PickleMachine) opcode_FLOAT() error {
	str, err := pm.readString()
	if err != nil {
		return err
	}
	var v float64
	_, err = fmt.Sscanf(str, "%f", &v)
	if err != nil {
		return err
	}
	pm.push(v)
	return nil
}

/**
Opcode: APPEND
Append an object to a list.

      Stack before:  ... pylist anyobject
      Stack after:   ... pylist+[anyobject]

      although pylist is really extended in-place.
      **
Stack before: [list, any]
Stack after: [list]
**/
func (pm *PickleMachine) opcode_APPEND() error {
	v, err := pm.pop()
	if err != nil {
		return err
	}

	listI, err := pm.pop()
	if err != nil {
		return err
	}

	list, ok := listI.([]interface{})
	if !ok {
		fmt.Errorf("Second item on top of stack must be of %T not %T", list, listI)
	}
	list = append(list, v)
	pm.push(list)
	return nil
}

/**
Opcode: LIST
Build a list out of the topmost stack slice, after markobject.

      All the stack entries following the topmost markobject are placed into
      a single Python list, which single list object replaces all of the
      stack from the topmost markobject onward.  For example,

      Stack before: ... markobject 1 2 3 'abc'
      Stack after:  ... [1, 2, 3, 'abc']
      **
Stack before: [mark, stackslice]
Stack after: [list]
**/
func (pm *PickleMachine) opcode_LIST() error {
	markIndex, err := pm.findMark()
	if err != nil {
		return err
	}
	v := make([]interface{}, 0)
	for i := markIndex + 1; i != len(pm.Stack); i++ {
		v = append(v, pm.Stack[i])
	}

	//Pop the values off the stack
	pm.popAfterIndex(markIndex)

	pm.push(v)
	return nil
}

/**
Opcode: TUPLE
Build a tuple out of the topmost stack slice, after markobject.

      All the stack entries following the topmost markobject are placed into
      a single Python tuple, which single tuple object replaces all of the
      stack from the topmost markobject onward.  For example,

      Stack before: ... markobject 1 2 3 'abc'
      Stack after:  ... (1, 2, 3, 'abc')
      **
Stack before: [mark, stackslice]
Stack after: [tuple]
**/
func (pm *PickleMachine) opcode_TUPLE() error {
	return pm.opcode_LIST()
}

/**
Opcode: DICT
Build a dict out of the topmost stack slice, after markobject.

      All the stack entries following the topmost markobject are placed into
      a single Python dict, which single dict object replaces all of the
      stack from the topmost markobject onward.  The stack slice alternates
      key, value, key, value, ....  For example,

      Stack before: ... markobject 1 2 3 'abc'
      Stack after:  ... {1: 2, 3: 'abc'}
      **
Stack before: [mark, stackslice]
Stack after: [dict]
**/
func (pm *PickleMachine) opcode_DICT() (err error) {
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

	v := make(map[interface{}]interface{})
	var key interface{}
	for i := markIndex + 1; i != len(pm.Stack); i++ {
		if key == nil {
			key = pm.Stack[i]
		} else {
			v[key] = pm.Stack[i]
			key = nil
		}
	}
	if key != nil {
		return fmt.Errorf("For opcode DICT stack after mark contained an odd number of items, this is not valid")
	}
	pm.popAfterIndex(markIndex)

	pm.push(v)
	return nil
}

/**
Opcode: SETITEM
Add a key+value pair to an existing dict.

      Stack before:  ... pydict key value
      Stack after:   ... pydict

      where pydict has been modified via pydict[key] = value.
      **
Stack before: [dict, any, any]
Stack after: [dict]
**/
func (pm *PickleMachine) opcode_SETITEM() (err error) {
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
	v, err := pm.pop()
	if err != nil {
		return err
	}

	k, err := pm.pop()
	if err != nil {
		return err
	}

	dictI, err := pm.pop()
	if err != nil {
		return err
	}

	dict, ok := dictI.(map[interface{}]interface{})
	if !ok {
		return fmt.Errorf("For opcode SETITEM stack item 2 from top must be of type %T not %T", dict, dictI)
	}

	dict[k] = v
	pm.push(dict)

	return nil
}

/**
Opcode: POP
Discard the top stack item, shrinking the stack by one item.**
Stack before: [any]
Stack after: []
**/
func (pm *PickleMachine) opcode_POP() error {
	_, err := pm.pop()
	return err

}

/**
Opcode: DUP
Push the top stack item onto the stack again, duplicating it.**
Stack before: [any]
Stack after: [any, any]
**/
func (pm *PickleMachine) opcode_DUP() error {
	return ErrOpcodeNotImplemented
}

/**
Opcode: MARK
Push markobject onto the stack.

      markobject is a unique object, used by other opcodes to identify a
      region of the stack containing a variable number of objects for them
      to work on.  See markobject.doc for more detail.
      **
Stack before: []
Stack after: [mark]
**/
func (pm *PickleMachine) opcode_MARK() error {
	pm.lastMark = len(pm.Stack)
	pm.push(PickleMark{})
	return nil
}

/**
Opcode: GET
Read an object from the memo and push it on the stack.

      The index of the memo object to push is given by the newline-terminated
      decimal string following.  BINGET and LONG_BINGET are space-optimized
      versions.
      **
Stack before: []
Stack after: [any]
**/
func (pm *PickleMachine) opcode_GET() error {
	str, err := pm.readString()
	if err != nil {
		return err
	}

	index, err := strconv.Atoi(str)
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
Opcode: PUT
Store the stack top into the memo.  The stack is not popped.

      The index of the memo location to write into is given by the newline-
      terminated decimal string following.  BINPUT and LONG_BINPUT are
      space-optimized versions.
      **
Stack before: []
Stack after: []
**/
func (pm *PickleMachine) opcode_PUT() error {
	if len(pm.Stack) < 1 {
		return ErrStackTooSmall
	}

	str, err := pm.readString()
	if err != nil {
		return err
	}

	idx, err := strconv.Atoi(str)
	if err != nil {
		return err
	}

	pm.storeMemo(int64(idx), pm.Stack[len(pm.Stack)-1])

	return nil
}

/**
Opcode: GLOBAL
Push a global object (module.attr) on the stack.

      Two newline-terminated strings follow the GLOBAL opcode.  The first is
      taken as a module name, and the second as a class name.  The class
      object module.class is pushed on the stack.  More accurately, the
      object returned by self.find_class(module, class) is pushed on the
      stack, so unpickling subclasses can override this form of lookup.
      **
Stack before: []
Stack after: [any]
**/
func (pm *PickleMachine) opcode_GLOBAL() error {
	//TODO push an object that represents the result of this operation
	return ErrOpcodeNotImplemented
}

/**
Opcode: REDUCE
Push an object built from a callable and an argument tuple.

      The opcode is named to remind of the __reduce__() method.

      Stack before: ... callable pytuple
      Stack after:  ... callable(*pytuple)

      The callable and the argument tuple are the first two items returned
      by a __reduce__ method.  Applying the callable to the argtuple is
      supposed to reproduce the original object, or at least get it started.
      If the __reduce__ method returns a 3-tuple, the last component is an
      argument to be passed to the object's __setstate__, and then the REDUCE
      opcode is followed by code to create setstate's argument, and then a
      BUILD opcode to apply  __setstate__ to that argument.

      If type(callable) is not ClassType, REDUCE complains unless the
      callable has been registered with the copy_reg module's
      safe_constructors dict, or the callable has a magic
      '__safe_for_unpickling__' attribute with a true value.  I'm not sure
      why it does this, but I've sure seen this complaint often enough when
      I didn't want to <wink>.
      **
Stack before: [any, any]
Stack after: [any]
**/
func (pm *PickleMachine) opcode_REDUCE() error {
	//TODO push an object that represents the result result of this operation
	return ErrOpcodeNotImplemented
}

/**
Opcode: BUILD
Finish building an object, via __setstate__ or dict update.

      Stack before: ... anyobject argument
      Stack after:  ... anyobject

      where anyobject may have been mutated, as follows:

      If the object has a __setstate__ method,

          anyobject.__setstate__(argument)

      is called.

      Else the argument must be a dict, the object must have a __dict__, and
      the object is updated via

          anyobject.__dict__.update(argument)

      This may raise RuntimeError in restricted execution mode (which
      disallows access to __dict__ directly); in that case, the object
      is updated instead via

          for k, v in argument.items():
              anyobject[k] = v
      **
Stack before: [any, any]
Stack after: [any]
**/
func (pm *PickleMachine) opcode_BUILD() error {
	return ErrOpcodeNotImplemented
}

/**
Opcode: INST
Build a class instance.

      This is the protocol 0 version of protocol 1's OBJ opcode.
      INST is followed by two newline-terminated strings, giving a
      module and class name, just as for the GLOBAL opcode (and see
      GLOBAL for more details about that).  self.find_class(module, name)
      is used to get a class object.

      In addition, all the objects on the stack following the topmost
      markobject are gathered into a tuple and popped (along with the
      topmost markobject), just as for the TUPLE opcode.

      Now it gets complicated.  If all of these are true:

        + The argtuple is empty (markobject was at the top of the stack
          at the start).

        + It's an old-style class object (the type of the class object is
          ClassType).

        + The class object does not have a __getinitargs__ attribute.

      then we want to create an old-style class instance without invoking
      its __init__() method (pickle has waffled on this over the years; not
      calling __init__() is current wisdom).  In this case, an instance of
      an old-style dummy class is created, and then we try to rebind its
      __class__ attribute to the desired class object.  If this succeeds,
      the new instance object is pushed on the stack, and we're done.  In
      restricted execution mode it can fail (assignment to __class__ is
      disallowed), and I'm not really sure what happens then -- it looks
      like the code ends up calling the class object's __init__ anyway,
      via falling into the next case.

      Else (the argtuple is not empty, it's not an old-style class object,
      or the class object does have a __getinitargs__ attribute), the code
      first insists that the class object have a __safe_for_unpickling__
      attribute.  Unlike as for the __safe_for_unpickling__ check in REDUCE,
      it doesn't matter whether this attribute has a true or false value, it
      only matters whether it exists (XXX this is a bug; cPickle
      requires the attribute to be true).  If __safe_for_unpickling__
      doesn't exist, UnpicklingError is raised.

      Else (the class object does have a __safe_for_unpickling__ attr),
      the class object obtained from INST's arguments is applied to the
      argtuple obtained from the stack, and the resulting instance object
      is pushed on the stack.

      NOTE:  checks for __safe_for_unpickling__ went away in Python 2.3.
      **
Stack before: [mark, stackslice]
Stack after: [any]
**/
func (pm *PickleMachine) opcode_INST() error {
	return ErrOpcodeNotImplemented
}

/**
Opcode: STOP
Stop the unpickling machine.

      Every pickle ends with this opcode.  The object at the top of the stack
      is popped, and that's the result of unpickling.  The stack should be
      empty then.
      **
Stack before: [any]
Stack after: []
**/
func (pm *PickleMachine) opcode_STOP() error {
	return ErrOpcodeStopped
}

/**
Opcode: PERSID
Push an object identified by a persistent ID.

      The pickle module doesn't define what a persistent ID means.  PERSID's
      argument is a newline-terminated str-style (no embedded escapes, no
      bracketing quote characters) string, which *is* "the persistent ID".
      The unpickler passes this string to self.persistent_load().  Whatever
      object that returns is pushed on the stack.  There is no implementation
      of persistent_load() in Python's unpickler:  it must be supplied by an
      unpickler subclass.
      **
Stack before: []
Stack after: [any]
**/
func (pm *PickleMachine) opcode_PERSID() error {
	return ErrOpcodeNotImplemented
}
