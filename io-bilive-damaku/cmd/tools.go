package main

import (
	"os"

	"github.com/urfave/cli/v2"
	"k8s.io/klog/v2"
)

func main() {
	app := &cli.App{
		Name:                 "Damaku Tools",
		Usage:                "Bilive Damaku Tools",
		HideHelpCommand:      true,
		EnableBashCompletion: true,
		Commands: []*cli.Command{
			WashApp.Command(),
		},
	}

	if err := app.Run(os.Args); err != nil {
		klog.Fatal(err)
	}
}
