package relevance

import (
	"context"
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"
)

func TestEvaluateReportsCandidatePrecisionRecallAndTrapLeakage(t *testing.T) {
	dir := t.TempDir()
	candidatePath := filepath.Join(dir, "candidates.parquet")
	scorePath := filepath.Join(dir, "scores.parquet")
	createEvaluationFixture(t, candidatePath, scorePath)

	labelsPath := filepath.Join(dir, "labels.csv")
	file, err := os.Create(labelsPath)
	if err != nil {
		t.Fatalf("create labels: %v", err)
	}
	writer := csv.NewWriter(file)
	if err := writer.Write([]string{
		"source_type",
		"source_id",
		"label_travel",
		"label_saas_opportunity",
		"label_app_opportunity",
		"false_positive_category",
	}); err != nil {
		t.Fatalf("write label header: %v", err)
	}
	rows := [][]string{
		{"comment", "retain-a", "1", "0", "0", ""},
		{"comment", "retain-b", "0", "0", "0", "generic_product_mention"},
		{"comment", "evaluate-a", "1", "0", "0", ""},
		{"comment", "evaluate-b", "0", "1", "1", ""},
		{"comment", "discard-a", "0", "0", "0", "payment_brand_visa"},
		{"comment", "discard-b", "0", "0", "0", "lexical_ambiguity"},
	}
	for _, row := range rows {
		if err := writer.Write(row); err != nil {
			t.Fatalf("write label row: %v", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		t.Fatalf("flush labels: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close labels: %v", err)
	}

	result, err := Evaluate(context.Background(), EvaluationOptions{
		LabelsPath:             labelsPath,
		ScorePath:              scorePath,
		ScriptPath:             filepath.Join("..", "..", "scripts", "dev", "duckdb_evaluate_relevance.py"),
		MinimumRetainPrecision: 0.70,
	})
	if err != nil {
		t.Fatalf("evaluate relevance: %v", err)
	}

	if result.RowsLabeled != 6 {
		t.Fatalf("expected 6 labels, got %d", result.RowsLabeled)
	}
	if result.Candidate.RetainedPredictions != 2 ||
		result.Candidate.TruePositiveRetained != 1 ||
		result.Candidate.RetainedPrecision != 0.5 {
		t.Fatalf("unexpected candidate metrics: %+v", result.Candidate)
	}
	if result.Candidate.LabeledPositive != 3 ||
		result.Candidate.RetainedRecall != 1.0/3.0 {
		t.Fatalf("unexpected candidate recall: %+v", result.Candidate)
	}
	if result.TrapLeakage["generic_product_mention"] != 1 {
		t.Fatalf("expected one retained trap, got %v", result.TrapLeakage)
	}
	if result.QualityGatePassed {
		t.Fatalf("expected 70%% precision gate to fail: %+v", result)
	}
}
