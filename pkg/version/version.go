package version

import "github.com/hashicorp/go-version"

var Version = "unknown"

func CompareVersion(v1, v2 string) (int, error) {
	version1, err := version.NewVersion(v1)
	if err != nil {
		return 0, err
	}
	version2, err := version.NewVersion(v2)
	if err != nil {
		return 0, err
	}

	if version1.LessThan(version2) {
		return -1, nil
	}
	if version1.GreaterThan(version2) {
		return 1, nil
	}
	return 0, nil
}
