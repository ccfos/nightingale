package runner

import (
	"hash/crc32"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/toolkits/pkg/file"
	"go.uber.org/automaxprocs/maxprocs"
)

var (
	Hostname string
	Cwd      string
)

func Noop(string, ...interface{}) {}

func Init() {

	maxprocs.Set(maxprocs.Logger(Noop))

	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	var err error
	Hostname, err = os.Hostname()
	if err != nil {
		log.Fatalln("[F] cannot get hostname")
	}

	Cwd = file.SelfDir()

	rand.Seed(time.Now().UnixNano() + int64(os.Getpid()+os.Getppid()) + int64(crc32.ChecksumIEEE([]byte(Hostname))))
}
