package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	telemetrypkg "goklipper/internal/pkg/motion/telemetry"
)

func main() {
	baselinePath := flag.String("baseline", "", "Path to baseline telemetry file (JSON array or NDJSON)")
	candidatePath := flag.String("candidate", "", "Path to candidate telemetry file (JSON array or NDJSON)")
	pretty := flag.Bool("pretty", true, "Pretty-print JSON output")
	flag.Parse()

	if *baselinePath == "" || *candidatePath == "" {
		fmt.Fprintln(os.Stderr, "usage: motion_telemetry_diff -baseline <baseline.json|ndjson> -candidate <candidate.json|ndjson> [-pretty=true|false]")
		os.Exit(2)
	}

	baselineFile, err := os.Open(*baselinePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open baseline telemetry: %v\n", err)
		os.Exit(1)
	}
	defer baselineFile.Close()
	candidateFile, err := os.Open(*candidatePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open candidate telemetry: %v\n", err)
		os.Exit(1)
	}
	defer candidateFile.Close()

	baselineSamples, err := telemetrypkg.ParseSamples(baselineFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse baseline telemetry: %v\n", err)
		os.Exit(1)
	}
	candidateSamples, err := telemetrypkg.ParseSamples(candidateFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse candidate telemetry: %v\n", err)
		os.Exit(1)
	}

	report := telemetrypkg.BuildDiffReport(baselineSamples, candidateSamples)
	encoder := json.NewEncoder(os.Stdout)
	if *pretty {
		encoder.SetIndent("", "  ")
	}
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintf(os.Stderr, "encode diff report: %v\n", err)
		os.Exit(1)
	}
}
