package stalecucumber

import "io"
import "reflect"
import "errors"
import "encoding/binary"
import "fmt"
import "math/big"

type pickleProxy interface {
	WriteTo(io.Writer) (int, error)
}

type Pickler struct {
	W io.Writer

	program []pickleProxy
}

/*
This type is used to pickle data.Picklers are created
by calling NewPickler. Each call to Pickle writes a
complete pickle program to the underlying io.Writer object.

Its safe to assign W to other values in between calls to Pickle.

Failures return the underlying error or an instance of PicklingError.

Data is always written using Pickle Protocol 2. This format is
compatible with Python 2.3 and all newer version.

Type Conversions

Type conversion from Go types to Python types is as follows

	uint8,uint16,int8,int16,int32 -> Python int
	int,int64,uint,uint64 -> Python int if it fits, otherwise Python Long
	string -> Python unicode
	slices, arrays -> Python list
	maps -> Python dict
	bool -> Python True and False
	big.Int -> Python Long
	struct -> Python dict

Structs are pickled using their field names unless a tag is present on the
field specifying the name. For example

	type MyType struct {
		FirstField int
		SecondField int `pickle:"meow"`
	}

This struct would be pickled into a dictionary with two keys: "FirstField"
and "meow".

Embedded structs are marshalled as a nested dictionary. Exported types
are never pickled.

Pickling Tuples

There is no equivalent type to Python's tuples in Go. You may not need
to use tuples at all. For example, consider the following Python code


	a, b, c = pickle.load(data_in)

This code tries to set to the variables "a", "b", and "c" from the result
of unpickling. In this case it does not matter if the source type
is a Python list or a Python tuple.

If you really need to write tuples, call NewTuple and pass the data
in as the arguments. This special type exists to inform stalecucumber.Pickle that
a tuple should be pickled.

*/
func NewPickler(writer io.Writer) *Pickler {
	retval := &Pickler{}
	retval.W = writer
	return retval
}

func (p *Pickler) Pickle(v interface{}) (int, error) {
	if p.program != nil {
		p.program = p.program[0:0]
	}

	err := p.dump(v)
	if err != nil {
		return 0, err
	}

	return p.writeProgram()
}

var programStart = []uint8{OPCODE_PROTO, 0x2}
var programEnd = []uint8{OPCODE_STOP}

func (p *Pickler) writeProgram() (n int, err error) {
	n, err = p.W.Write(programStart)
	if err != nil {
		return
	}
	var m int
	for _, proxy := range p.program {
		m, err = proxy.WriteTo(p.W)
		if err != nil {
			return
		}

		n += m
	}

	m, err = p.W.Write(programEnd)
	if err != nil {
		return
	}
	n += m

	return
}

const BININT_MAX = (1 << 31) - 1
const BININT_MIN = 0 - BININT_MAX

var ErrTypeNotPickleable = errors.New("Can't pickle this type")
var ErrEmptyInterfaceNotPickleable = errors.New("The empty interface is not pickleable")

type PicklingError struct {
	V   interface{}
	Err error
}

func (pe PicklingError) Error() string {
	return fmt.Sprintf("Failed pickling (%T)%v:%v", pe.V, pe.V, pe.Err)
}

func (p *Pickler) dump(input interface{}) error {
	if input == nil {
		p.pushOpcode(OPCODE_NONE)
		return nil
	}

	switch input := input.(type) {
	case int:
		if input <= BININT_MAX && input >= BININT_MIN {
			p.dumpInt(int64(input))
			return nil
		}
		p.dumpIntAsLong(int64(input))
		return nil
	case int64:
		if input <= BININT_MAX && input >= BININT_MIN {
			p.dumpInt(input)
			return nil
		}
		p.dumpIntAsLong(input)
		return nil
	case int8:
		p.dumpInt(int64(input))
		return nil
	case int16:
		p.dumpInt(int64(input))
		return nil
	case int32:
		p.dumpInt(int64(input))
		return nil

	case uint8:
		p.dumpInt(int64(input))
		return nil
	case uint16:
		p.dumpInt(int64(input))
		return nil

	case uint32:
		if input <= BININT_MAX {
			p.dumpInt(int64(input))
			return nil
		}
		p.dumpUintAsLong(uint64(input))
		return nil

	case uint:
		if input <= BININT_MAX {
			p.dumpInt(int64(input))
			return nil
		}
		p.dumpUintAsLong(uint64(input))
		return nil
	case uint64:
		if input <= BININT_MAX {
			p.dumpInt(int64(input))
			return nil
		}
		p.dumpUintAsLong(input)
		return nil
	case float32:
		p.dumpFloat(float64(input))
		return nil
	case float64:
		p.dumpFloat(input)
		return nil
	case string:
		p.dumpString(input)
		return nil
	case bool:
		p.dumpBool(input)
		return nil
	case big.Int:
		p.dumpBigInt(input)
		return nil
	case PickleNone:
		p.pushOpcode(OPCODE_NONE)
		return nil
	case PickleTuple:
		l := len(input)
		switch l {
		case 0:
			p.pushOpcode(OPCODE_EMPTY_TUPLE)
			return nil
		case 1, 2, 3:
		default:
			p.pushOpcode(OPCODE_MARK)
		}

		for _, v := range input {
			err := p.dump(v)
			if err != nil {
				return err
			}
		}

		switch l {
		case 1:
			p.pushOpcode(OPCODE_TUPLE1)
		case 2:
			p.pushOpcode(OPCODE_TUPLE2)
		case 3:
			p.pushOpcode(OPCODE_TUPLE3)
		default:
			p.pushOpcode(OPCODE_TUPLE)
		}

		return nil
	}

	v := reflect.ValueOf(input)
	vKind := v.Kind()

	switch vKind {
	//Check for pointers. They can't be
	//meaningfully written as a pickle unless nil. Dereference
	//and recurse.
	case reflect.Ptr:
		if v.IsNil() {
			p.pushOpcode(OPCODE_NONE)
			return nil
		}
		return p.dump(v.Elem().Interface())
	case reflect.Map:
		p.pushOpcode(OPCODE_EMPTY_DICT)
		p.pushOpcode(OPCODE_MARK)

		keys := v.MapKeys()
		for _, key := range keys {
			err := p.dump(key.Interface())
			if err != nil {
				return err
			}
			val := v.MapIndex(key)
			err = p.dump(val.Interface())
			if err != nil {
				return err
			}
		}
		p.pushOpcode(OPCODE_SETITEMS)
		return nil
	case reflect.Slice, reflect.Array:
		p.pushOpcode(OPCODE_EMPTY_LIST)
		p.pushOpcode(OPCODE_MARK)
		for i := 0; i != v.Len(); i++ {
			element := v.Index(i)
			p.dump(element.Interface())
		}
		p.pushOpcode(OPCODE_APPENDS)
		return nil
	case reflect.Struct:
		return p.dumpStruct(input)
	}

	return PicklingError{V: input, Err: ErrTypeNotPickleable}
}

func (p *Pickler) dumpBool(v bool) {
	if v {
		p.pushOpcode(OPCODE_NEWTRUE)
	} else {
		p.pushOpcode(OPCODE_NEWFALSE)
	}
}

func (p *Pickler) dumpStruct(input interface{}) error {
	vType := reflect.TypeOf(input)
	v := reflect.ValueOf(input)

	p.pushOpcode(OPCODE_EMPTY_DICT)
	p.pushOpcode(OPCODE_MARK)

	for i := 0; i != v.NumField(); i++ {
		field := vType.Field(i)
		//Never attempt to write
		//unexported names
		if len(field.PkgPath) != 0 {
			continue
		}

		//Prefer the tagged name of the
		//field, fall back to fields actual name
		fieldKey := field.Tag.Get(PICKLE_TAG)
		if len(fieldKey) == 0 {
			fieldKey = field.Name
		}
		p.dumpString(fieldKey)

		fieldValue := v.Field(i)
		err := p.dump(fieldValue.Interface())
		if err != nil {
			return err
		}

	}
	p.pushOpcode(OPCODE_SETITEMS)
	return nil
}

func (p *Pickler) pushProxy(proxy pickleProxy) {
	p.program = append(p.program, proxy)
}

func (p *Pickler) dumpFloat(v float64) {
	p.pushProxy(floatProxy(v))
}

type opcodeProxy uint8

func (proxy opcodeProxy) WriteTo(w io.Writer) (int, error) {
	return w.Write([]byte{byte(proxy)})
}

func (p *Pickler) pushOpcode(code uint8) {
	p.pushProxy(opcodeProxy(code))
}

type bigIntProxy struct {
	v *big.Int
}

var zeroPad = []byte{0}
var maxPad = []byte{0xff}

func (proxy bigIntProxy) WriteTo(w io.Writer) (int, error) {
	var negative = proxy.v.Sign() == -1
	var raw []byte
	if negative {
		offset := big.NewInt(1)

		bitLen := uint(proxy.v.BitLen())
		remainder := bitLen % 8
		bitLen += 8 - remainder

		offset.Lsh(offset, bitLen)

		offset.Add(proxy.v, offset)

		raw = offset.Bytes()
	} else {
		raw = proxy.v.Bytes()
	}

	var pad []byte
	var padL int
	var highBitSet = (raw[0] & 0x80) != 0

	if negative && !highBitSet {
		pad = maxPad
		padL = 1
	} else if !negative && highBitSet {
		pad = zeroPad
		padL = 1
	}

	l := len(raw)
	var header interface{}
	if l < 256 {
		header = struct {
			Opcode uint8
			Length uint8
		}{
			OPCODE_LONG1,
			uint8(l + padL),
		}
	} else {
		header = struct {
			Opcode uint8
			Length uint32
		}{
			OPCODE_LONG4,
			uint32(l + padL),
		}
	}

	err := binary.Write(w, binary.LittleEndian, header)
	if err != nil {
		return 0, err
	}

	n := binary.Size(header)
	n += l
	n += padL

	reversed := make([]byte, l)

	for i, v := range raw {
		reversed[l-i-1] = v
	}

	_, err = w.Write(reversed)
	if err != nil {
		return n, err
	}

	_, err = w.Write(pad)

	return n, err
}

func (p *Pickler) dumpIntAsLong(v int64) {
	p.pushProxy(bigIntProxy{big.NewInt(v)})
}

func (p *Pickler) dumpBigInt(v big.Int) {
	p.pushProxy(bigIntProxy{&v}) //Note that this is a shallow copy
}

func (p *Pickler) dumpUintAsLong(v uint64) {
	w := big.NewInt(0)
	w.SetUint64(v)
	p.pushProxy(bigIntProxy{w})
}

type floatProxy float64

func (proxy floatProxy) WriteTo(w io.Writer) (int, error) {
	data := struct {
		Opcode uint8
		V      float64
	}{
		OPCODE_BINFLOAT,
		float64(proxy),
	}

	return binary.Size(data), binary.Write(w, binary.BigEndian, data)
}

type intProxy int32

func (proxy intProxy) WriteTo(w io.Writer) (int, error) {
	data := struct {
		Opcode uint8
		V      int32
	}{
		OPCODE_BININT,
		int32(proxy),
	}

	return binary.Size(data), binary.Write(w, binary.LittleEndian, data)
}

func (p *Pickler) dumpInt(v int64) {
	p.pushProxy(intProxy(v))
}

type stringProxy string

func (proxy stringProxy) V() interface{} {
	return proxy
}

func (proxy stringProxy) WriteTo(w io.Writer) (int, error) {
	header := struct {
		Opcode uint8
		Length int32
	}{
		OPCODE_BINUNICODE,
		int32(len(proxy)),
	}
	err := binary.Write(w, binary.LittleEndian, header)
	if err != nil {
		return 0, err
	}
	n := binary.Size(header)

	m, err := io.WriteString(w, string(proxy))
	if err != nil {
		return 0, err
	}

	return n + m, nil

}

func (p *Pickler) dumpString(v string) {
	p.pushProxy(stringProxy(v))
}
