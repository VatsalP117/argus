package relevance

import (
	"context"
	"encoding/csv"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestEvaluateReportsCandidatePrecisionRecallAndTrapLeakage(t *testing.T) {
	dir := t.TempDir()
	candidatePath := filepath.Join(dir, "candidates.parquet")
	scorePath := filepath.Join(dir, "scores.parquet")
	createEvaluationFixture(t, candidatePath, scorePath)

	labelsPath := writeLabels(t, dir, []labelRow{
		{"retain-a", "1", "0", "0", "", "", "https://reddit.test/retain-a"},
		{"retain-b", "0", "0", "0", "generic_product_mention", "", "https://reddit.test/retain-b"},
		{"evaluate-a", "1", "0", "0", "", "", "https://reddit.test/evaluate-a"},
		{"evaluate-b", "0", "1", "1", "", "", "https://reddit.test/evaluate-b"},
		{"discard-a", "0", "0", "0", "payment_brand_visa", "", "https://reddit.test/discard-a"},
		{"discard-b", "0", "0", "0", "lexical_ambiguity", "", "https://reddit.test/discard-b"},
	})

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
		result.Candidate.FixtureRecall != 1.0/3.0 {
		t.Fatalf("unexpected candidate fixture recall: %+v", result.Candidate)
	}
	if result.Candidate.ExactRetainedPrecision != result.Candidate.RetainedPrecision {
		t.Fatalf("exact retained precision should equal retained precision: %+v", result.Candidate)
	}
	if result.TrapLeakage["generic_product_mention"] != 1 {
		t.Fatalf("expected one retained trap, got %v", result.TrapLeakage)
	}
	if result.QualityGatePassed {
		t.Fatalf("expected 70%% precision gate to fail: %+v", result)
	}
}

func TestEvaluateWeightedRecallUsesStratumPopulations(t *testing.T) {
	dir := t.TempDir()
	candidatePath := filepath.Join(dir, "candidates.parquet")
	scorePath := filepath.Join(dir, "scores.parquet")
	createEvaluationFixture(t, candidatePath, scorePath)

	labelsPath := writeLabels(t, dir, []labelRow{
		{"retain-a", "1", "0", "0", "", "", "https://reddit.test/retain-a"},
		{"retain-b", "0", "0", "0", "", "", "https://reddit.test/retain-b"},
		{"evaluate-a", "1", "0", "0", "", "", "https://reddit.test/evaluate-a"},
		{"evaluate-b", "0", "1", "1", "", "", "https://reddit.test/evaluate-b"},
		{"discard-a", "0", "0", "0", "", "", "https://reddit.test/discard-a"},
		{"discard-b", "0", "0", "0", "", "", "https://reddit.test/discard-b"},
	})

	result, err := Evaluate(context.Background(), EvaluationOptions{
		LabelsPath:             labelsPath,
		ScorePath:              scorePath,
		ScriptPath:             filepath.Join("..", "..", "scripts", "dev", "duckdb_evaluate_relevance.py"),
		MinimumRetainPrecision: 0.70,
	})
	if err != nil {
		t.Fatalf("evaluate relevance: %v", err)
	}

	if result.Strata["retain"].Population != 2 || result.Strata["retain"].SampleSize != 2 {
		t.Fatalf("unexpected retain stratum: %+v", result.Strata["retain"])
	}
	if result.Strata["evaluate"].Population != 2 || result.Strata["evaluate"].SampleSize != 2 {
		t.Fatalf("unexpected evaluate stratum: %+v", result.Strata["evaluate"])
	}
	if result.Strata["discard"].Population != 2 || result.Strata["discard"].SampleSize != 2 {
		t.Fatalf("unexpected discard stratum: %+v", result.Strata["discard"])
	}

	expectedEstimatedRelevant := 2.0*result.Strata["retain"].SampledPositiveRate +
		2.0*result.Strata["evaluate"].SampledPositiveRate +
		2.0*result.Strata["discard"].SampledPositiveRate
	if result.Candidate.EstimatedTotalRelevant != expectedEstimatedRelevant {
		t.Fatalf("expected estimated relevant %f, got %f", expectedEstimatedRelevant, result.Candidate.EstimatedTotalRelevant)
	}
	expectedWeightedRecall := float64(result.Candidate.TruePositiveRetained) / expectedEstimatedRelevant
	if result.Candidate.WeightedRetainedRecallEstimate != expectedWeightedRecall {
		t.Fatalf("expected weighted recall %f, got %f", expectedWeightedRecall, result.Candidate.WeightedRetainedRecallEstimate)
	}
}

func TestEvaluateRejectsMissingPopulationMetadata(t *testing.T) {
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
	for _, row := range [][]string{
		{"comment", "retain-a", "1", "0", "0", ""},
		{"comment", "retain-b", "0", "0", "0", ""},
	} {
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

	_, err = Evaluate(context.Background(), EvaluationOptions{
		LabelsPath:             labelsPath,
		ScorePath:              scorePath,
		ScriptPath:             filepath.Join("..", "..", "scripts", "dev", "duckdb_evaluate_relevance.py"),
		MinimumRetainPrecision: 0.70,
	})
	if err == nil {
		t.Fatalf("expected error for missing population metadata")
	}
}

func TestEvaluateRejectsIncompleteRetainStratum(t *testing.T) {
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
		"sample_stratum",
		"stratum_population",
		"sample_rank",
		"sampling_seed",
		"label_travel",
		"label_saas_opportunity",
		"label_app_opportunity",
		"false_positive_category",
		"label_notes",
		"source_url",
	}); err != nil {
		t.Fatalf("write label header: %v", err)
	}
	for _, row := range [][]string{
		{"comment", "retain-a", "retain", "2", "1", "fixture", "1", "0", "0", "", "", "https://reddit.test/retain-a"},
		{"comment", "evaluate-a", "evaluate", "2", "1", "fixture", "1", "0", "0", "", "", "https://reddit.test/evaluate-a"},
		{"comment", "evaluate-b", "evaluate", "2", "2", "fixture", "0", "1", "1", "", "", "https://reddit.test/evaluate-b"},
		{"comment", "discard-a", "discard", "2", "1", "fixture", "0", "0", "0", "", "", "https://reddit.test/discard-a"},
		{"comment", "discard-b", "discard", "2", "2", "fixture", "0", "0", "0", "", "", "https://reddit.test/discard-b"},
	} {
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

	_, err = Evaluate(context.Background(), EvaluationOptions{
		LabelsPath:             labelsPath,
		ScorePath:              scorePath,
		ScriptPath:             filepath.Join("..", "..", "scripts", "dev", "duckdb_evaluate_relevance.py"),
		MinimumRetainPrecision: 0.70,
	})
	if err == nil {
		t.Fatalf("expected error for incomplete retain stratum")
	}
}

type labelRow struct {
	SourceID              string
	Travel                string
	SaaS                  string
	App                   string
	FalsePositiveCategory string
	Notes                 string
	SourceURL             string
}

func writeLabels(t *testing.T, dir string, rows []labelRow) string {
	t.Helper()
	labelsPath := filepath.Join(dir, "labels.csv")
	file, err := os.Create(labelsPath)
	if err != nil {
		t.Fatalf("create labels: %v", err)
	}
	writer := csv.NewWriter(file)
	if err := writer.Write([]string{
		"source_type",
		"source_id",
		"sample_stratum",
		"stratum_population",
		"sample_rank",
		"sampling_seed",
		"label_travel",
		"label_saas_opportunity",
		"label_app_opportunity",
		"false_positive_category",
		"label_notes",
		"source_url",
	}); err != nil {
		t.Fatalf("write label header: %v", err)
	}
	populations := map[string]int{
		"retain":   0,
		"evaluate": 0,
		"discard":  0,
	}
	stratumRanks := map[string]int{}
	for _, row := range rows {
		populations[stratumForSourceID(row.SourceID)]++
	}
	for _, row := range rows {
		stratum := stratumForSourceID(row.SourceID)
		stratumRanks[stratum]++
		record := []string{
			"comment",
			row.SourceID,
			stratum,
			strconv.Itoa(populations[stratum]),
			strconv.Itoa(stratumRanks[stratum]),
			"fixture",
			row.Travel,
			row.SaaS,
			row.App,
			row.FalsePositiveCategory,
			row.Notes,
			row.SourceURL,
		}
		if err := writer.Write(record); err != nil {
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
	return labelsPath
}

func stratumForSourceID(sourceID string) string {
	switch {
	case sourceID == "retain-a" || sourceID == "retain-b":
		return "retain"
	case sourceID == "evaluate-a" || sourceID == "evaluate-b":
		return "evaluate"
	default:
		return "discard"
	}
}
