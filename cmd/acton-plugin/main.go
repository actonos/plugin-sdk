package main

import (
	"fmt"
	"os"
)

const version = "1.0.0"

func printUsage() {
	fmt.Print(`
   ___       __              ____  _____   _____ ___  __ __
  / _ | ____/ /____  ___    / __ \/ ___/  / __// _ \/ //_/
 / __ |/ __/ __/ _ \/ _ \  / /_/ /\__ \  _\ \ / // / ,<   
/_/ |_|\__/\__/\___/_//_/  \____//____/ /___//____/_/|_|  
ActonOS Plugin Developer CLI (v` + version + `)

USAGE:
  acton-plugin <command> [arguments]

COMMANDS:
  new <name>       Scaffold a new plugin project (tool, channel, or connector)
  build            Compile the plugin to WebAssembly (GOOS=wasip1 GOARCH=wasm)
  build-all        Batch compile and package all plugins in plugins/ to dist/
  validate         Validate manifest.json schema, permissions, and security
  test             Execute plugin verification on local Wazero Mock Host
  sign             Sign plugin bundle using Ed25519 cryptographic key
  pack             Package plugin into a distribution bundle (.actonpkg)
  version          Display CLI version info

EXAMPLES:
  acton-plugin new weather-tool --type=tool
  acton-plugin new telegram-bot --type=channel
  acton-plugin new github-sync  --type=connector
  acton-plugin build
  acton-plugin build-all
  acton-plugin validate
  acton-plugin test
  acton-plugin pack

Run 'acton-plugin <command> --help' for details on specific commands.
`)
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "new", "init":
		err = runNew(args)
	case "build-all", "pack-all":
		err = runBuildAll(args)
	case "build":
		err = runBuild(args)
	case "validate":
		err = runValidate(args)
	case "test":
		err = runTest(args)
	case "sign":
		err = runSign(args)
	case "pack", "package":
		err = runPack(args)
	case "version", "-v", "--version":
		fmt.Printf("acton-plugin CLI version %s\n", version)
		return
	case "help", "-h", "--help":
		printUsage()
		return
	default:
		fmt.Fprintf(os.Stderr, "Unknown command '%s'. Run 'acton-plugin help' for usage.\n", cmd)
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
