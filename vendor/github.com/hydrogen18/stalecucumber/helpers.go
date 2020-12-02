package stalecucumber

import "fmt"
import "math/big"

/*
This type is returned whenever a helper cannot convert the result of
Unpickle into the desired type.
*/
type WrongTypeError struct {
	Result  interface{}
	Request string
}

func (wte WrongTypeError) Error() string {
	return fmt.Sprintf("Unpickling returned type %T which cannot be converted to %s", wte.Result, wte.Request)
}

func newWrongTypeError(result interface{}, request interface{}) error {
	return WrongTypeError{Result: result, Request: fmt.Sprintf("%T", request)}
}

/*
This helper attempts to convert the return value of Unpickle into a string.

If Unpickle returns an error that error is returned immediately.

If the value cannot be converted an error is returned.
*/
func String(v interface{}, err error) (string, error) {
	if err != nil {
		return "", err
	}
	vs, ok := v.(string)
	if ok {
		return vs, nil
	}

	return "", newWrongTypeError(v, vs)
}

/*
This helper attempts to convert the return value of Unpickle into a int64.

If Unpickle returns an error that error is returned immediately.

If the value cannot be converted an error is returned.
*/
func Int(v interface{}, err error) (int64, error) {
	if err != nil {
		return 0, err
	}
	vi, ok := v.(int64)
	if ok {
		return vi, nil
	}

	vbi, ok := v.(*big.Int)
	if ok {
		if vbi.BitLen() <= 63 {
			return vbi.Int64(), nil
		}
	}

	return 0, newWrongTypeError(v, vi)

}

/*
This helper attempts to convert the return value of Unpickle into a bool.

If Unpickle returns an error that error is returned immediately.

If the value cannot be converted an error is returned.
*/
func Bool(v interface{}, err error) (bool, error) {
	if err != nil {
		return false, err
	}
	vb, ok := v.(bool)
	if ok {
		return vb, nil
	}

	return false, newWrongTypeError(v, vb)

}

/*
This helper attempts to convert the return value of Unpickle into a *big.Int.

If Unpickle returns an error that error is returned immediately.

If the value cannot be converted an error is returned.
*/
func Big(v interface{}, err error) (*big.Int, error) {
	if err != nil {
		return nil, err
	}

	vb, ok := v.(*big.Int)
	if ok {
		return vb, nil
	}

	return nil, newWrongTypeError(v, vb)

}

/*
This helper attempts to convert the return value of Unpickle into a float64.

If Unpickle returns an error that error is returned immediately.

If the value cannot be converted an error is returned.
*/
func Float(v interface{}, err error) (float64, error) {
	if err != nil {
		return 0.0, err
	}

	vf, ok := v.(float64)
	if ok {
		return vf, nil
	}

	return 0.0, newWrongTypeError(v, vf)
}

/*
This helper attempts to convert the return value of Unpickle into a []interface{}.

If Unpickle returns an error that error is returned immediately.

If the value cannot be converted an error is returned.
*/
func ListOrTuple(v interface{}, err error) ([]interface{}, error) {
	if err != nil {
		return nil, err
	}

	vl, ok := v.([]interface{})
	if ok {
		return vl, nil
	}

	return nil, newWrongTypeError(v, vl)

}

/*
This helper attempts to convert the return value of Unpickle into a map[interface{}]interface{}.

If Unpickle returns an error that error is returned immediately.

If the value cannot be converted an error is returned.
*/
func Dict(v interface{}, err error) (map[interface{}]interface{}, error) {
	if err != nil {
		return nil, err
	}

	vd, ok := v.(map[interface{}]interface{})
	if ok {
		return vd, nil
	}

	return nil, newWrongTypeError(v, vd)
}

/*
This helper attempts to convert the return value of Unpickle into a map[string]interface{}.

If Unpickle returns an error that error is returned immediately.

If the value cannot be converted an error is returned.
*/
func DictString(v interface{}, err error) (map[string]interface{}, error) {
	var src map[interface{}]interface{}
	src, err = Dict(v, err)
	if err != nil {
		return nil, err
	}

	return tryDictToDictString(src)
}

func tryDictToDictString(src map[interface{}]interface{}) (map[string]interface{}, error) {
	dst := make(map[string]interface{}, len(src))

	for k, v := range src {
		kstr, ok := k.(string)
		if !ok {
			return nil, newWrongTypeError(src, dst)
		}
		dst[kstr] = v
	}

	return dst, nil

}
