package base64Captcha

import "io"

//Item is captcha item interface
type Item interface {
	//WriteTo writes to a writer
	WriteTo(w io.Writer) (n int64, err error)
	//EncodeB64string encodes as base64 string
	EncodeB64string() string
}
