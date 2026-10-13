package main

import (
	"bytes"
	"reflect"
	"sync"
	"testing"
)

func TestPackageProgressStages(t *testing.T) {
	for _, test := range []struct{ line, stage string }{
		{"npm http fetch GET 200 https://registry.npmjs.org/example 200ms", "Downloading"},
		{"npm http cache https://registry.npmjs.org/example 0ms (cache hit)", ""},
		{"Progress: resolved 5, reused 2, downloaded 0, added 0", "Resolving"},
		{"Progress: resolved 5, reused 2, downloaded 3, added 0", "Downloading"},
		{"Progress: resolved 5, reused 2, downloaded 3, added 5, done", "Installing"},
		{"\x1b[32mResolving dependencies\x1b[0m", "Resolving"},
		{"Downloading [2/4]", "Downloading"},
		{"Extracting [1/4]", "Extracting"},
		{"Linking dependencies", "Linking"},
		{".../node_modules/example: Running postinstall script...", "Building"},
		{"installed example@1.0.0 with binaries:", "Installing"},
		{"removed 1 package in 1s", "Removing"},
		{"warn deprecated downloading-helper@1.0.0", ""},
		{"error: request failed", ""},
	} {
		t.Run(test.line, func(t *testing.T) {
			if got := packageProgressStage(test.line); got != test.stage {
				t.Fatalf("stage = %q, want %q", got, test.stage)
			}
		})
	}
}

func TestProgressWriterPreservesOutputAndHandlesChunks(t *testing.T) {
	var output bytes.Buffer
	var stages []string
	reporter := &packageProgressReporter{emit: func(stage string) { stages = append(stages, stage) }}
	writer := &packageProgressWriter{output: &output, reporter: reporter}
	chunks := []string{"Resol", "ving dependencies\r\nResolving dependencies\nDown", "loading [1/2]\rInstalling"}
	for _, chunk := range chunks {
		if n, err := writer.Write([]byte(chunk)); n != len(chunk) || err != nil {
			t.Fatalf("Write = %d, %v", n, err)
		}
	}
	writer.flush()
	if want := []string{"Resolving", "Downloading", "Installing"}; !reflect.DeepEqual(stages, want) {
		t.Fatalf("stages = %v, want %v", stages, want)
	}
	if output.String() != "Resolving dependencies\r\nResolving dependencies\nDownloading [1/2]\rInstalling" {
		t.Fatalf("command output changed: %q", output.String())
	}
}

func TestProgressReporterSerializesOutputStreams(t *testing.T) {
	var stages []string
	reporter := &packageProgressReporter{emit: func(stage string) { stages = append(stages, stage) }}
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				reporter.report("Downloading [1/2]")
			}
		}()
	}
	wg.Wait()
	if !reflect.DeepEqual(stages, []string{"Downloading"}) {
		t.Fatalf("duplicate progress events: %v", stages)
	}
}
