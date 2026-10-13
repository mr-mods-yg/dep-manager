package main

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

const packageProgressEvent = "package:progress"

type PackageActionProgress struct {
	Manager     string `json:"manager"`
	PackageName string `json:"packageName"`
	Action      string `json:"action"`
	Stage       string `json:"stage"`
}

// stdout and stderr may arrive concurrently; emit only stage changes.
type packageProgressReporter struct {
	mu   sync.Mutex
	last string
	emit func(string)
}

func (r *packageProgressReporter) report(line string) {
	stage := packageProgressStage(line)
	if stage == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if stage != r.last {
		r.last = stage
		r.emit(stage)
	}
}

// Preserve command output while parsing complete lines, including CLI redraws.
type packageProgressWriter struct {
	output   *bytes.Buffer
	reporter *packageProgressReporter
	pending  string
}

func (w *packageProgressWriter) Write(data []byte) (int, error) {
	n, err := w.output.Write(data)
	w.pending += string(data)
	for {
		end := strings.IndexAny(w.pending, "\r\n")
		if end < 0 {
			break
		}
		w.reporter.report(w.pending[:end])
		w.pending = w.pending[end+1:]
	}
	// Bound partial lines from noisy commands without affecting saved output.
	if len(w.pending) > 8192 {
		w.pending = w.pending[len(w.pending)-8192:]
	}
	return n, err
}

func (w *packageProgressWriter) flush() {
	w.reporter.report(w.pending)
	w.pending = ""
}

var terminalEscape = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
var pnpmProgress = regexp.MustCompile(`^progress: resolved (\d+), reused \d+, downloaded (\d+), added (\d+)`)

func packageProgressStage(line string) string {
	line = strings.ToLower(strings.TrimSpace(terminalEscape.ReplaceAllString(line, "")))
	if counts := pnpmProgress.FindStringSubmatch(line); counts != nil {
		added, _ := strconv.Atoi(counts[3])
		downloaded, _ := strconv.Atoi(counts[2])
		if added > 0 {
			return "Installing"
		}
		if downloaded > 0 {
			return "Downloading"
		}
		return "Resolving"
	}
	switch {
	case strings.HasPrefix(line, "npm http fetch "):
		return "Downloading"
	case strings.HasPrefix(line, "resolving"), strings.HasPrefix(line, "resolved "):
		return "Resolving"
	case strings.HasPrefix(line, "downloading"), strings.HasPrefix(line, "downloaded "):
		return "Downloading"
	case strings.HasPrefix(line, "extracting"), strings.HasPrefix(line, "extracted "):
		return "Extracting"
	case strings.HasPrefix(line, "linking"):
		return "Linking"
	case strings.HasPrefix(line, "building"), strings.Contains(line, ": running postinstall script"):
		return "Building"
	case strings.HasPrefix(line, "installing"), strings.HasPrefix(line, "installed "), strings.HasPrefix(line, "added "):
		return "Installing"
	case strings.HasPrefix(line, "removing"), strings.HasPrefix(line, "removed "):
		return "Removing"
	default:
		return ""
	}
}
