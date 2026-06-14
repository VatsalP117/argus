package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRequiresLabelsAndScores(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run(nil, &stdout, &stderr)

	if exitCode != 2 {
		t.Fatalf("expected usage exit code 2, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "labels and score-path are required") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}
