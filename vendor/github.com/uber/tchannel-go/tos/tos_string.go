package tos

import "fmt"

var (
	_tosNameToValue map[string]ToS
	_tosValueToName = map[ToS]string{
		CS3:         "CS3",
		CS4:         "CS4",
		CS5:         "CS5",
		CS6:         "CS6",
		CS7:         "CS7",
		AF11:        "AF11",
		AF12:        "AF12",
		AF13:        "AF13",
		AF21:        "AF21",
		AF22:        "AF22",
		AF23:        "AF23",
		AF31:        "AF31",
		AF32:        "AF32",
		AF33:        "AF33",
		AF41:        "AF41",
		AF42:        "AF42",
		AF43:        "AF43",
		EF:          "EF",
		Lowdelay:    "Lowdelay",
		Throughput:  "Throughput",
		Reliability: "Reliability",
		Lowcost:     "Lowcost",
	}
)

func init() {
	_tosNameToValue = make(map[string]ToS, len(_tosValueToName))
	for tos, tosString := range _tosValueToName {
		_tosNameToValue[tosString] = tos
	}
}

// MarshalText implements TextMarshaler from encoding
func (r ToS) MarshalText() ([]byte, error) {
	return []byte(_tosValueToName[r]), nil
}

// UnmarshalText implements TextUnMarshaler from encoding
func (r *ToS) UnmarshalText(data []byte) error {
	if v, ok := _tosNameToValue[string(data)]; ok {
		*r = v
		return nil
	}

	return fmt.Errorf("invalid ToS %q", string(data))
}
