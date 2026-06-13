package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"argus/internal/archive"
	"argus/internal/config"
	"argus/internal/manifest"
	"argus/internal/runmeta"
)

func main() {
	var pipelinePath string
	var outputPath string
	var month string
	var recordType string

	flag.StringVar(&pipelinePath, "pipeline", "configs/pipelines/pilot-travel-q1-2021.yaml", "path to pipeline config")
	flag.StringVar(&outputPath, "output", "", "path to output manifest json")
	flag.StringVar(&month, "month", "", "optional single month filter, format YYYY-MM")
	flag.StringVar(&recordType, "record-type", "", "optional single record type filter")
	flag.Parse()

	cfg, err := config.LoadPipelineConfig(pipelinePath)
	if err != nil {
		panic(err)
	}

	months := cfg.Months
	if month != "" {
		months = []string{month}
	}

	recordTypes := cfg.RecordTypes
	if recordType != "" {
		recordTypes = []string{recordType}
	}

	if outputPath == "" {
		outputPath = filepath.Join("manifests", "pilot", cfg.PipelineName+"-manifest.json")
	}

	client := archive.NewHFClient()
	archiveRevision, err := client.ResolveRevision(cfg.Archive.Repo)
	if err != nil {
		panic(err)
	}
	shardsByGroup := map[string][]archive.TreeFile{}

	for _, rt := range recordTypes {
		for _, m := range months {
			shards, err := client.ListMonthShardsAtRevision(cfg.Archive.Repo, archiveRevision, rt, m)
			if err != nil {
				panic(err)
			}
			shardsByGroup[rt+":"+m] = shards
		}
	}

	man := manifest.Build(cfg.Archive.Repo, archiveRevision, cfg, months, recordTypes, shardsByGroup)
	if err := manifest.Write(outputPath, man); err != nil {
		panic(err)
	}

	rec := runmeta.RunRecord{
		RunID:          "phase2-manifest-" + time.Now().UTC().Format("20060102T150405.000000000Z"),
		Phase:          "phase2",
		JobName:        "manifest_builder",
		StartedAt:      man.GeneratedAt,
		FinishedAt:     time.Now().UTC().Format(time.RFC3339),
		Status:         "completed",
		GitSHA:         gitSHA(),
		ConfigHash:     configHash(pipelinePath),
		RecordsSeen:    int64(man.Summary.EntryCount),
		RecordsWritten: int64(man.Summary.EntryCount),
		InputRefs:      []string{pipelinePath},
		OutputRefs:     []string{outputPath},
		Notes:          fmt.Sprintf("Built manifest with %d entries and %d bytes", man.Summary.EntryCount, man.Summary.BytesTotal),
	}

	runPath := filepath.Join(cfg.State.RunsDir, "phase2", rec.RunID+".json")
	if err := runmeta.Write(runPath, rec); err != nil {
		panic(err)
	}

	fmt.Printf("manifest written: %s\n", outputPath)
	fmt.Printf("entries: %d\n", man.Summary.EntryCount)
	fmt.Printf("bytes_total: %d\n", man.Summary.BytesTotal)
}

func gitSHA() string {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return string(bytesTrimSpace(out))
}

func configHash(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "unavailable"
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func bytesTrimSpace(in []byte) []byte {
	start := 0
	end := len(in)
	for start < end && (in[start] == ' ' || in[start] == '\n' || in[start] == '\r' || in[start] == '\t') {
		start++
	}
	for end > start && (in[end-1] == ' ' || in[end-1] == '\n' || in[end-1] == '\r' || in[end-1] == '\t') {
		end--
	}
	return in[start:end]
}
