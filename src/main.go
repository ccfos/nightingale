package main

import (
	"fmt"
	"os"

	"github.com/toolkits/pkg/runner"
	"github.com/urfave/cli/v2"

	"github.com/didi/nightingale/v5/src/pkg/version"
	"github.com/didi/nightingale/v5/src/server"
	"github.com/didi/nightingale/v5/src/webapi"
)

func main() {
	app := cli.NewApp()
	app.Name = "n9e"
	app.Version = version.VERSION
	app.Usage = "Nightingale, enterprise prometheus management"
	app.Commands = []*cli.Command{
		newWebapiCmd(),
		newServerCmd(),
	}
	app.Run(os.Args)
}

func newWebapiCmd() *cli.Command {
	return &cli.Command{
		Name:  "webapi",
		Usage: "Run webapi",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "conf",
				Aliases: []string{"c"},
				Usage:   "specify configuration file(.json,.yaml,.toml)",
			},
			&cli.StringFlag{
				Name:    "key",
				Aliases: []string{"k"},
				Usage:   "specify the secret key for configuration file field encryption",
			},
		},
		Action: func(c *cli.Context) error {
			printEnv()

			var opts []webapi.WebapiOption
			if c.String("conf") != "" {
				opts = append(opts, webapi.SetConfigFile(c.String("conf")))
			}
			opts = append(opts, webapi.SetVersion(version.VERSION))
			if c.String("key") != "" {
				opts = append(opts, webapi.SetKey(c.String("key")))
			}

			webapi.Run(opts...)
			return nil
		},
	}
}

func newServerCmd() *cli.Command {
	return &cli.Command{
		Name:  "server",
		Usage: "Run server",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "conf",
				Aliases: []string{"c"},
				Usage:   "specify configuration file(.json,.yaml,.toml)",
			},
			&cli.StringFlag{
				Name:    "key",
				Aliases: []string{"k"},
				Usage:   "specify the secret key for configuration file field encryption",
			},
		},
		Action: func(c *cli.Context) error {
			printEnv()

			var opts []server.ServerOption
			if c.String("conf") != "" {
				opts = append(opts, server.SetConfigFile(c.String("conf")))
			}
			opts = append(opts, server.SetVersion(version.VERSION))
			if c.String("key") != "" {
				opts = append(opts, server.SetKey(c.String("key")))
			}

			server.Run(opts...)
			return nil
		},
	}
}

func printEnv() {
	runner.Init()
	fmt.Println("runner.cwd:", runner.Cwd)
	fmt.Println("runner.hostname:", runner.Hostname)
	fmt.Println("runner.fd_limits:", runner.FdLimits())
	fmt.Println("runner.vm_limits:", runner.VMLimits())
}
