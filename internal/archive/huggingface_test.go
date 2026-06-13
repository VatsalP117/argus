package archive

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestResolveRevisionReturnsDatasetCommitSHA(t *testing.T) {
	client := &HFClient{
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.URL.String() != "https://huggingface.co/api/datasets/open-index/arctic" {
					t.Fatalf("unexpected request url: %s", request.URL)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"sha":"revision-123"}`)),
					Header:     make(http.Header),
				}, nil
			}),
		},
	}

	revision, err := client.ResolveRevision("open-index/arctic")
	if err != nil {
		t.Fatalf("resolve revision: %v", err)
	}
	if revision != "revision-123" {
		t.Fatalf("unexpected revision: %s", revision)
	}
}

func TestListMonthShardsAtRevisionUsesPinnedTreeAndPreservesOID(t *testing.T) {
	client := &HFClient{
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				expected := "https://huggingface.co/api/datasets/open-index/arctic/tree/revision-123/data/comments/2021/01?recursive=false&expand=false"
				if request.URL.String() != expected {
					t.Fatalf("unexpected request url: %s", request.URL)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(
						`[{"type":"file","path":"data/comments/2021/01/000.parquet","size":10,"oid":"object-abc"}]`,
					)),
					Header: make(http.Header),
				}, nil
			}),
		},
	}

	shards, err := client.ListMonthShardsAtRevision("open-index/arctic", "revision-123", "comments", "2021-01")
	if err != nil {
		t.Fatalf("list shards: %v", err)
	}
	if len(shards) != 1 {
		t.Fatalf("expected one shard, got %d", len(shards))
	}
	if shards[0].OID != "object-abc" {
		t.Fatalf("unexpected source oid: %s", shards[0].OID)
	}
}
