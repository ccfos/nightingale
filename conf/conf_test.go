package conf

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
)

func TestInitConfig(t *testing.T) {
	{
		patches := gomonkey.ApplyFunc(os.Stat, func(name string) (fs.FileInfo, error) {
			return nil, os.ErrNotExist
		})

		patchesExit := gomonkey.ApplyFunc(os.Exit, func(code int) {
			return
		})

		defer func() {
			patchesExit.Reset()
			patches.Reset()
		}()

		_, err := InitConfig("non_exist_dir", "test_key")
		fmt.Println(err)
	}

	{
		patches := gomonkey.ApplyFunc(os.Stat, func(name string) (fs.FileInfo, error) {
			return nil, nil
		})

		patchesWalk := gomonkey.ApplyFunc(filepath.Walk, func(root string, fn filepath.WalkFunc) error {
			return nil
		})

		patchesExit := gomonkey.ApplyFunc(os.Exit, func(code int) {
			return
		})

		defer func() {
			patches.Reset()
			patchesWalk.Reset()
			patchesExit.Reset()
		}()

		_, err := InitConfig("exist_dir", "test_key")
		fmt.Println(err)
	}
}
