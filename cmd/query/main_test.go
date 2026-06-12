package main

import "testing"

func TestNormalizeFilter(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty becomes wildcard", input: "", want: "*"},
		{name: "whitespace becomes wildcard", input: "   ", want: "*"},
		{name: "lowercases text", input: " Pain_Point ", want: "pain_point"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeFilter(tt.input); got != tt.want {
				t.Fatalf("normalizeFilter(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildOutputRefs(t *testing.T) {
	if refs := buildOutputRefs(""); refs != nil {
		t.Fatalf("expected nil refs for empty output path, got %#v", refs)
	}

	refs := buildOutputRefs("data/exports/query.json")
	if len(refs) != 1 || refs[0] != "data/exports/query.json" {
		t.Fatalf("unexpected output refs: %#v", refs)
	}
}
