package gosnmp

import (
	"bytes"
	"fmt"
	l "github.com/alouca/gologger"
	"strconv"
	"strings"
)

type SnmpVersion uint8

const (
	Version1  SnmpVersion = 0x0
	Version2c SnmpVersion = 0x1
)

func (s SnmpVersion) String() string {
	if s == Version1 {
		return "1"
	} else if s == Version2c {
		return "2c"
	}
	return "U"
}

type SnmpPacket struct {
	Version        SnmpVersion
	Community      string
	RequestType    Asn1BER
	RequestID      uint8
	Error          uint8
	ErrorIndex     uint8
	NonRepeaters   uint8
	MaxRepetitions uint8
	Variables      []SnmpPDU
}

type SnmpPDU struct {
	Name  string
	Type  Asn1BER
	Value interface{}
}

func Unmarshal(packet []byte) (*SnmpPacket, error) {
	log := l.GetDefaultLogger()

	log.Debug("Begin SNMP Packet unmarshal\n")

	//var err error
	response := new(SnmpPacket)
	response.Variables = make([]SnmpPDU, 0, 5)

	// Start parsing the packet
	var cursor uint64 = 0

	// First bytes should be 0x30
	if Asn1BER(packet[0]) == Sequence {
		// Parse packet length
		ber, err := parseField(packet)

		if err != nil {
			log.Error("Unable to parse packet header: %s\n", err.Error())
			return nil, err
		}

		log.Debug("Packet sanity verified, we got all the bytes (%d)\n", ber.DataLength)

		cursor += ber.HeaderLength
		// Parse SNMP Version
		rawVersion, err := parseField(packet[cursor:])

		if err != nil {
			return nil, fmt.Errorf("Error parsing SNMP packet version: %s", err.Error())
		}

		cursor += rawVersion.DataLength + rawVersion.HeaderLength
		if version, ok := rawVersion.BERVariable.Value.(int); ok {
			response.Version = SnmpVersion(version)
			log.Debug("Parsed Version %d\n", version)
		}

		// Parse community
		rawCommunity, err := parseField(packet[cursor:])
		if err != nil {
			log.Debug("Unable to parse Community Field: %s\n", err)
		}
		cursor += rawCommunity.DataLength + rawCommunity.HeaderLength

		if community, ok := rawCommunity.BERVariable.Value.(string); ok {
			response.Community = community
			log.Debug("Parsed community %s\n", community)
		}

		rawPDU, err := parseField(packet[cursor:])

		if err != nil {
			log.Debug("Unable to parse SNMP PDU: %s\n", err.Error())
		}
		response.RequestType = rawPDU.Type

		switch rawPDU.Type {
		default:
			log.Debug("Unsupported SNMP Packet Type %s\n", rawPDU.Type.String())
			log.Debug("PDU Size is %d\n", rawPDU.DataLength)
		case GetRequest, GetResponse, GetBulkRequest:
			log.Debug("SNMP Packet is %s\n", rawPDU.Type.String())
			log.Debug("PDU Size is %d\n", rawPDU.DataLength)
			cursor += rawPDU.HeaderLength

			// Parse Request ID
			rawRequestId, err := parseField(packet[cursor:])

			if err != nil {
				return nil, err
			}

			cursor += rawRequestId.DataLength + rawRequestId.HeaderLength
			if requestid, ok := rawRequestId.BERVariable.Value.(int); ok {
				response.RequestID = uint8(requestid)
				log.Debug("Parsed Request ID: %d\n", requestid)
			}

			// Parse Error
			rawError, err := parseField(packet[cursor:])

			if err != nil {
				return nil, err
			}

			cursor += rawError.DataLength + rawError.HeaderLength
			if errorNo, ok := rawError.BERVariable.Value.(int); ok {
				response.Error = uint8(errorNo)
			}

			// Parse Error Index
			rawErrorIndex, err := parseField(packet[cursor:])

			if err != nil {
				return nil, err
			}

			cursor += rawErrorIndex.DataLength + rawErrorIndex.HeaderLength

			if errorindex, ok := rawErrorIndex.BERVariable.Value.(int); ok {
				response.ErrorIndex = uint8(errorindex)
			}

			log.Debug("Request ID: %d Error: %d Error Index: %d\n", response.RequestID, response.Error, response.ErrorIndex)
			rawResp, err := parseField(packet[cursor:])

			if err != nil {
				return nil, err
			}

			cursor += rawResp.HeaderLength
			// Loop & parse Varbinds
			for cursor < uint64(len(packet)) {
				log.Debug("Parsing var bind response (Cursor at %d/%d)", cursor, len(packet))

				rawVarbind, err := parseField(packet[cursor:])

				if err != nil {
					return nil, err
				}

				cursor += rawVarbind.HeaderLength
				log.Debug("Varbind length: %d/%d\n", rawVarbind.HeaderLength, rawVarbind.DataLength)

				log.Debug("Parsing OID (Cursor at %d)\n", cursor)
				// Parse OID
				rawOid, err := parseField(packet[cursor:])

				if err != nil {
					return nil, err
				}

				cursor += rawOid.HeaderLength + rawOid.DataLength

				log.Debug("OID (%v) Field was %d bytes\n", rawOid, rawOid.DataLength)

				rawValue, err := parseField(packet[cursor:])

				if err != nil {
					return nil, err
				}
				cursor += rawValue.HeaderLength + rawValue.DataLength

				log.Debug("Value field was %d bytes\n", rawValue.DataLength)

				if oid, ok := rawOid.BERVariable.Value.([]int); ok {
					log.Debug("Varbind decoding success\n")
					response.Variables = append(response.Variables, SnmpPDU{oidToString(oid), rawValue.Type, rawValue.BERVariable.Value})
				}
			}

		}
	} else {
		return nil, fmt.Errorf("Invalid packet header\n")
	}

	return response, nil
}

type RawBER struct {
	Type         Asn1BER
	HeaderLength uint64
	DataLength   uint64
	Data         []byte
	BERVariable  *Variable
}

// Parses a given field, return the ASN.1 BER Type, its header length and the data
func parseField(data []byte) (*RawBER, error) {
	log := l.GetDefaultLogger()
	var err error

	if len(data) == 0 {
		return nil, fmt.Errorf("Unable to parse BER: Data length 0")
	}

	ber := new(RawBER)

	ber.Type = Asn1BER(data[0])

	// Parse Length
	length := data[1]

	// Check if this is padded or not
	if length > 0x80 {
		length = length - 0x80
		log.Debug("Field length is padded to %d bytes\n", length)
		ber.DataLength = Uvarint(data[2 : 2+length])
		log.Debug("Decoded final length: %d\n", ber.DataLength)

		ber.HeaderLength = 2 + uint64(length)

	} else {
		ber.HeaderLength = 2
		ber.DataLength = uint64(length)
	}

	// Do sanity checks
	if ber.DataLength > uint64(len(data)) {
		return nil, fmt.Errorf("Unable to parse BER: provided data length is longer than actual data (%d vs %d)", ber.DataLength, len(data))
	}

	ber.Data = data[ber.HeaderLength : ber.HeaderLength+ber.DataLength]

	ber.BERVariable, err = decodeValue(ber.Type, ber.Data)

	if err != nil {
		return nil, fmt.Errorf("Unable to decode value: %s\n", err.Error())
	}

	return ber, nil
}

func (packet *SnmpPacket) marshal() ([]byte, error) {
	// Prepare the buffer to send
	buffer := make([]byte, 0, 1024)
	buf := bytes.NewBuffer(buffer)

	// Write the packet header (Message type 0x30) & Version = 2
	buf.Write([]byte{byte(Sequence), 0, 2, 1, byte(packet.Version)})

	// Write Community
	buf.Write([]byte{4, uint8(len(packet.Community))})
	buf.WriteString(packet.Community)

	// Marshal the SNMP PDU
	snmpPduBuffer := make([]byte, 0, 1024)
	snmpPduBuf := bytes.NewBuffer(snmpPduBuffer)

	snmpPduBuf.Write([]byte{byte(packet.RequestType), 0, 2, 1, packet.RequestID})

	switch packet.RequestType {
	case GetBulkRequest:
		snmpPduBuf.Write([]byte{
			2, 1, packet.NonRepeaters,
			2, 1, packet.MaxRepetitions,
		})
	default:
		snmpPduBuf.Write([]byte{
			2, 1, packet.Error,
			2, 1, packet.ErrorIndex,
		})
	}

	snmpPduBuf.Write([]byte{byte(Sequence), 0})

	pduLength := 0
	for _, varlist := range packet.Variables {
		pdu, err := marshalPDU(&varlist)

		if err != nil {
			return nil, err
		}
		pduLength += len(pdu)
		snmpPduBuf.Write(pdu)
	}

	pduBytes := snmpPduBuf.Bytes()
	// Varbind list length
	pduBytes[12] = byte(pduLength)
	// SNMP PDU length (PDU header + varbind list length)
	pduBytes[1] = byte(pduLength + 11)

	buf.Write(pduBytes)

	// Write the
	//buf.Write([]byte{packet.RequestType, uint8(17 + len(mOid)), 2, 1, 1, 2, 1, 0, 2, 1, 0, 0x30, uint8(6 + len(mOid)), 0x30, uint8(4 + len(mOid)), 6, uint8(len(mOid))})
	//buf.Write(mOid)
	//buf.Write([]byte{5, 0})

	ret := buf.Bytes()

	// Set the packet size
	ret[1] = uint8(len(ret) - 2)

	return ret, nil
}

func marshalPDU(pdu *SnmpPDU) ([]byte, error) {
	oid, err := marshalOID(pdu.Name)
	if err != nil {
		return nil, err
	}

	pduBuffer := make([]byte, 0, 1024)
	pduBuf := bytes.NewBuffer(pduBuffer)

	// Mashal the PDU type into the appropriate BER
	switch pdu.Type {
	case Null:
		pduBuf.Write([]byte{byte(Sequence), byte(len(oid) + 4)})
		pduBuf.Write([]byte{byte(ObjectIdentifier), byte(len(oid))})
		pduBuf.Write(oid)
		pduBuf.Write([]byte{Null, 0x00})
	default:
		return nil, fmt.Errorf("Unable to marshal PDU: unknown BER type %d", pdu.Type)
	}

	return pduBuf.Bytes(), nil
}

func oidToString(oid []int) (ret string) {
	values := make([]interface{}, len(oid))
	for i, v := range oid {
		values[i] = v
	}
	return fmt.Sprintf(strings.Repeat(".%d", len(oid)), values...)
}

func marshalOID(oid string) ([]byte, error) {
	var err error

	// Encode the oid
	oid = strings.Trim(oid, ".")
	oidParts := strings.Split(oid, ".")
	oidBytes := make([]int, len(oidParts))

	// Convert the string OID to an array of integers
	for i := 0; i < len(oidParts); i++ {
		oidBytes[i], err = strconv.Atoi(oidParts[i])
		if err != nil {
			return nil, fmt.Errorf("Unable to parse OID: %s\n", err.Error())
		}
	}

	mOid, err := marshalObjectIdentifier(oidBytes)

	if err != nil {
		return nil, fmt.Errorf("Unable to marshal OID: %s\n", err.Error())
	}

	return mOid, err
}
