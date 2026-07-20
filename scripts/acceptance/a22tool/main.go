package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(arguments []string, stdout, stderr io.Writer) int {
	if len(arguments) == 0 {
		fmt.Fprintln(stderr, "usage: a22tool <seed|snapshot|report|validate> [arguments]")
		return 2
	}
	var err error
	switch arguments[0] {
	case "seed":
		err = runSeed(arguments[1:])
	case "snapshot":
		err = runSnapshot(arguments[1:])
	case "report":
		err = runReport(arguments[1:])
	case "validate":
		err = runValidate(arguments[1:])
	default:
		fmt.Fprintf(stderr, "unknown a22tool command %q\n", arguments[0])
		return 2
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func parseNoPositionals(name string, arguments []string, configure func(*flag.FlagSet)) (*flag.FlagSet, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configure(flags)
	if err := flags.Parse(arguments); err != nil {
		return nil, err
	}
	if flags.NArg() != 0 {
		return nil, fmt.Errorf("%s does not accept positional arguments", name)
	}
	return flags, nil
}
