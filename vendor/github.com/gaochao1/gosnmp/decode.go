// Copyright 2012 Andreas Louca. All rights reserved.
// Use of this source code is goverend by a BSD-style
// license that can be found in the LICENSE file.

package gosnmp

import (
	"fmt"
	"net"
)

type Asn1BER byte

// SNMP Data Types
const (
	Integer          Asn1BER = 0x02
	BitString                = 0x03
	OctetString              = 0x04
	Null                     = 0x05
	ObjectIdentifier         = 0x06
	Sequence                 = 0x30
	IpAddress                = 0x40
	Counter32                = 0x41
	Gauge32                  = 0x42
	TimeTicks                = 0x43
	Opaque                   = 0x44
	NsapAddress              = 0x45
	Counter64                = 0x46
	Uinteger32               = 0x47
	NoSuchObject             = 0x80
	NoSuchInstance           = 0x81
	GetRequest               = 0xa0
	GetNextRequest           = 0xa1
	GetResponse              = 0xa2
	SetRequest               = 0xa3
	Trap                     = 0xa4
	GetBulkRequest           = 0xa5
	EndOfMibView             = 0x82
)

// String representations of each SNMP Data Type
var dataTypeStrings = map[Asn1BER]string{
	Integer:          "Integer",
	BitString:        "BitString",
	OctetString:      "OctetString",
	Null:             "Null",
	ObjectIdentifier: "ObjectIdentifier",
	IpAddress:        "IpAddress",
	Sequence:         "Sequence",
	Counter32:        "Counter32",
	Gauge32:          "Gauge32",
	TimeTicks:        "TimeTicks",
	Opaque:           "Opaque",
	NsapAddress:      "NsapAddress",
	Counter64:        "Counter64",
	Uinteger32:       "Uinteger32",
	NoSuchObject:     "NoSuchObject",
	NoSuchInstance:   "NoSuchInstance",
	GetRequest:       "GetRequest",
	GetNextRequest:   "GetNextRequest",
	GetResponse:      "GetResponse",
	SetRequest:       "SetRequest",
	Trap:             "Trap",
	GetBulkRequest:   "GetBulkRequest",
	EndOfMibView:     "endOfMib",
}

func (dataType Asn1BER) String() string {
	str, ok := dataTypeStrings[dataType]

	if !ok {
		str = "Unknown"
	}

	return str
}

type Variable struct {
	Name  []int
	Type  Asn1BER
	Size  uint64
	Value interface{}
}

func decodeValue(valueType Asn1BER, data []byte) (retVal *Variable, err error) {
	retVal = new(Variable)
	retVal.Size = uint64(len(data))

	switch Asn1BER(valueType) {

	// Integer
	case Integer:
		ret, err := parseInt(data)
		if err != nil {
			break
		}
		retVal.Type = Integer
		retVal.Value = ret
	// Octet
	case OctetString:
		retVal.Type = OctetString
		retVal.Value = string(data)
	case ObjectIdentifier:
		retVal.Type = ObjectIdentifier
		retVal.Value, _ = parseObjectIdentifier(data)
	// IpAddress
	case IpAddress:
		retVal.Type = IpAddress
		retVal.Value = net.IP{data[0], data[1], data[2], data[3]}
	// Counter32
	case Counter32:
		ret, err := parseInt(data)
		if err != nil {
			break
		}
		retVal.Type = Counter32
		retVal.Value = ret
	case TimeTicks:
		ret, err := parseInt(data)
		if err != nil {
			break
		}
		retVal.Type = TimeTicks
		retVal.Value = ret
	// Gauge32
	case Gauge32:
		ret, err := parseInt(data)
		if err != nil {
			break
		}
		retVal.Type = Gauge32
		retVal.Value = ret
	case Counter64:
		ret, err := parseInt64(data)

		// Decode it
		if err != nil {
			break
		}

		retVal.Type = Counter64
		retVal.Value = ret
	case Null:
		retVal.Value = nil
	case Sequence:
		// NOOP
		retVal.Value = data
	case GetResponse:
		// NOOP
		retVal.Value = data
	case GetRequest:
		// NOOP
		retVal.Value = data
	case EndOfMibView:
		retVal.Type = EndOfMibView
		retVal.Value = "endOfMib"
	case GetBulkRequest:
		// NOOP
		retVal.Value = data
	case NoSuchInstance:
		return nil, fmt.Errorf("No such instance")
	case NoSuchObject:
		return nil, fmt.Errorf("No such object")
	default:
		err = fmt.Errorf("Unable to decode %s %#v - not implemented", valueType, valueType)
	}

	return retVal, err
}

// Parses UINT16
func ParseUint16(content []byte) int {
	number := uint8(content[1]) | uint8(content[0])<<8
	//fmt.Printf("\t%d\n", number)

	return int(number)
}
