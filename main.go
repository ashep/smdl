package main

import (
	"fmt"
	"os"

	"github.com/ashep/go-app/runner"
	"github.com/ashep/smdl/internal/app"
)

func main() {
	err := runner.New(app.Run).
		LoadEnvConfig().
		LoadConfigFile("config.yml").
		AddConsoleLogWriter().
		AddHTTPLogWriter().
		Run()

	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
