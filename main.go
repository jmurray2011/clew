package main

import "github.com/jmurray2011/clew/cmd"

// version is set via ldflags at build time
var version = "dev"

func main() {
	cmd.SetVersion(version)
	cmd.Execute()
}
