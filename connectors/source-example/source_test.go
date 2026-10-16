package main

import (
	"os"
	"path/filepath"
	"testing"

	airbyte "github.com/saturn4er/airbyte-go-sdk"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

func noopLog(airbyte.LogLevel, string) error { return nil }

func TestCheckRejectsOutOfRangeCount(t *testing.T) {
	src := NewExampleSource()
	logs := airbyte.LogTracker{Log: noopLog}

	if err := src.Check(writeConfig(t, `{"count": 5}`), logs); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	if err := src.Check(writeConfig(t, `{"count": -1}`), logs); err == nil {
		t.Fatal("negative count accepted")
	}
}

func TestReadEmitsConfiguredRows(t *testing.T) {
	src := NewExampleSource()

	catalog, err := src.Discover("", airbyte.LogTracker{Log: noopLog})
	if err != nil {
		t.Fatal(err)
	}

	configured := &airbyte.ConfiguredCatalog{Streams: []airbyte.ConfiguredStream{{Stream: catalog.Streams[0]}}}

	var ids []any

	tracker := airbyte.MessageTracker{
		Log: noopLog,
		Record: func(v any, stream, _ string) error {
			if stream != streamName {
				t.Fatalf("unexpected stream %q", stream)
			}

			ids = append(ids, v.(map[string]any)["id"])

			return nil
		},
	}

	if err := src.Read(writeConfig(t, `{"count": 3}`), "", configured, tracker); err != nil {
		t.Fatal(err)
	}

	if len(ids) != 3 || ids[0] != 1 || ids[2] != 3 {
		t.Fatalf("unexpected records: %v", ids)
	}
}
