package archive

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"
)

type TreeFile struct {
	Type string `json:"type"`
	Path string `json:"path"`
	Size int64  `json:"size"`
	OID  string `json:"oid"`
}

type HFClient struct {
	HTTPClient *http.Client
}

type datasetInfo struct {
	SHA string `json:"sha"`
}

func NewHFClient() *HFClient {
	return &HFClient{
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *HFClient) ResolveRevision(repo string) (string, error) {
	apiURL := fmt.Sprintf("https://huggingface.co/api/datasets/%s", repo)
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("hugging face dataset api returned status %d for %s", resp.StatusCode, apiURL)
	}

	var info datasetInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", err
	}
	if strings.TrimSpace(info.SHA) == "" {
		return "", fmt.Errorf("hugging face dataset api returned an empty revision for %s", repo)
	}
	return info.SHA, nil
}

func (c *HFClient) ListMonthShards(repo, recordType, month string) ([]TreeFile, error) {
	return c.ListMonthShardsAtRevision(repo, "main", recordType, month)
}

func (c *HFClient) ListMonthShardsAtRevision(repo, revision, recordType, month string) ([]TreeFile, error) {
	parts := strings.Split(month, "-")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid month %q, expected YYYY-MM", month)
	}

	apiURL := fmt.Sprintf(
		"https://huggingface.co/api/datasets/%s/tree/%s/data/%s/%s/%s?recursive=false&expand=false",
		repo,
		url.PathEscape(revision),
		url.PathEscape(recordType),
		url.PathEscape(parts[0]),
		url.PathEscape(parts[1]),
	)

	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hugging face tree api returned status %d for %s", resp.StatusCode, apiURL)
	}

	var tree []TreeFile
	if err := json.NewDecoder(resp.Body).Decode(&tree); err != nil {
		return nil, err
	}

	var shards []TreeFile
	for _, item := range tree {
		if item.Type != "file" {
			continue
		}
		if !strings.HasSuffix(item.Path, ".parquet") {
			continue
		}
		shards = append(shards, item)
	}

	sort.Slice(shards, func(i, j int) bool {
		return shards[i].Path < shards[j].Path
	})

	return shards, nil
}

func ResolveURL(repo, shardPath string) string {
	return fmt.Sprintf("https://huggingface.co/datasets/%s/resolve/main/%s", repo, shardPath)
}

func ResolveURLAtRevision(repo, revision, shardPath string) string {
	return fmt.Sprintf("https://huggingface.co/datasets/%s/resolve/%s/%s", repo, revision, shardPath)
}

func ShardName(shardPath string) string {
	return path.Base(shardPath)
}
