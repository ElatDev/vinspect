// Command corpus rebuilds the verification corpus in testdata/corpus from
// NHTSA's public APIs. It has three steps:
//
//	go run ./tools/corpus crash    # VINs of every vehicle in NHTSA's crash-test database
//	go run ./tools/corpus vpic     # NHTSA vPIC's own decode of each of those VINs
//	go run ./tools/corpus report   # compare vinspect's offline answers with vPIC's
//
// The first two steps are slow and network-bound, so every response is cached
// under .cache/ and a rerun resumes where the last one stopped. Their outputs
// are committed, which lets `go test ./...` re-check the corpus claims in CI
// without touching the network.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

func main() {
	log.SetFlags(log.Ltime)
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: corpus crash|vpic|report [flags]")
	}
	flag.Parse()
	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(2)
	}
	var err error
	switch cmd, args := flag.Arg(0), flag.Args()[1:]; cmd {
	case "crash":
		err = runCrash(args)
	case "vpic":
		err = runVPIC(args)
	case "report":
		err = runReport(args)
	default:
		flag.Usage()
		os.Exit(2)
	}
	if err != nil {
		log.Fatal(err)
	}
}
