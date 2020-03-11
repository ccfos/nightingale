package nux

import (
	"fmt"
)

// return: [][$fs_spec, $fs_file, $fs_vfstype]
func ListMountPoint() ([][3]string, error) {
	return [][3]string{}, nil
}

func BuildDeviceUsage(_fsSpec, _fsFile, _fsVfstype string) (*DeviceUsage, error) {
	return nil, fmt.Errorf("not supported")
}
