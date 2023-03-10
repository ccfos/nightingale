package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ccfos/nightingale/v6/alert"
	"github.com/ccfos/nightingale/v6/pkg/osx"
	"github.com/ccfos/nightingale/v6/pkg/version"

	"github.com/toolkits/pkg/runner"
)

var (
	showVersion = flag.Bool("version", false, "Show version.")
	configDir   = flag.String("configs", osx.GetEnv("N9E_CONFIGS", "etc"), "Specify configuration directory.(env:N9E_CONFIGS)")
	cryptoKey   = flag.String("crypto-key", "", "Specify the secret key for configuration file field encryption.")
)

func main() {
	flag.Parse()

	if *showVersion {
		fmt.Println(version.Version)
		os.Exit(0)
	}

	printEnv()

	cleanFunc, err := alert.Initialize(*configDir, *cryptoKey)
	if err != nil {
		log.Fatalln("failed to initialize:", err)
	}

	code := 1
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

EXIT:
	for {
		sig := <-sc
		fmt.Println("received signal:", sig.String())
		switch sig {
		case syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT:
			code = 0
			break EXIT
		case syscall.SIGHUP:
			// reload configuration?
		default:
			break EXIT
		}
	}

	cleanFunc()
	fmt.Println("process exited")
	os.Exit(code)
}

func printEnv() {
	runner.Init()
	fmt.Println("runner.cwd:", runner.Cwd)
	fmt.Println("runner.hostname:", runner.Hostname)
	fmt.Println("runner.fd_limits:", runner.FdLimits())
	fmt.Println("runner.vm_limits:", runner.VMLimits())
}
