package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	inputPath := flag.String("i", "", "input config file path")
	outputPath := flag.String("o", "", "output file path")
	preservePlaintext := flag.Bool("p", false, "preserve plaintext output")
	flag.Parse()

	fmt.Fprintf(os.Stderr, "config_convert has not been migrated yet. input=%q output=%q preserve_plaintext=%t\n", *inputPath, *outputPath, *preservePlaintext)
}
