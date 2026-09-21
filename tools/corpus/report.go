package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ElatDev/vinspect/internal/corpus"
	"github.com/ElatDev/vinspect/wmi"
)

func runReport(args []string) error {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	crash := fs.String("crash", "testdata/corpus/crash-test-vehicles.csv", "crash-test corpus CSV")
	decodes := fs.String("vpic", "testdata/corpus/vpic-decodes.csv", "vPIC decode CSV")
	readme := fs.String("readme", "", "rewrite the corpus block in this file instead of printing it")
	detail := fs.Bool("detail", false, "also list every disagreement")
	fs.Parse(args)

	rep, retrieved, err := corpus.Load(*crash, *decodes)
	if err != nil {
		return err
	}
	block := rep.Markdown(retrieved, wmi.Retrieved())
	if *detail {
		block += rep.Detail()
	}
	if *readme == "" {
		fmt.Print(block)
		return nil
	}

	src, err := os.ReadFile(*readme)
	if err != nil {
		return err
	}
	updated, err := replaceBlock(string(src), block)
	if err != nil {
		return fmt.Errorf("%s: %w", *readme, err)
	}
	if updated == string(src) {
		fmt.Println("corpus block in", *readme, "is already current")
		return nil
	}
	if err := os.WriteFile(*readme, []byte(updated), 0o644); err != nil {
		return err
	}
	fmt.Println("updated the corpus block in", *readme)
	return nil
}

// replaceBlock swaps the text between the corpus markers for block.
func replaceBlock(src, block string) (string, error) {
	start := strings.Index(src, corpus.MarkerStart)
	end := strings.Index(src, corpus.MarkerEnd)
	if start < 0 || end < start {
		return "", fmt.Errorf("missing %s ... %s markers", corpus.MarkerStart, corpus.MarkerEnd)
	}
	return src[:start] + strings.TrimSuffix(block, "\n") + src[end+len(corpus.MarkerEnd):], nil
}
