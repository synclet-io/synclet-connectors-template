package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"reflect"
	"testing"
)

// TestImageSpecMatchesSpecJSON builds the connector image and checks that its
// `spec` command reports what spec.json holds.
func TestImageSpecMatchesSpecJSON(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the connector image in Docker; skipped under -short")
	}

	const image = "local/source-example:test"

	build := exec.CommandContext(t.Context(), "docker", "build", "-q", "-t", image, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("docker build: %v\n%s", err, out)
	}

	out, err := exec.CommandContext(t.Context(), "docker", "run", "--rm", "--network", "none", image, "spec").Output()
	if err != nil {
		t.Fatalf("docker run spec: %v", err)
	}

	var got any

	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Buffer(nil, 1<<20)

	for scanner.Scan() {
		var msg struct {
			Type string          `json:"type"`
			Spec json.RawMessage `json:"spec"`
		}
		if json.Unmarshal(scanner.Bytes(), &msg) == nil && msg.Type == "SPEC" {
			if err := json.Unmarshal(msg.Spec, &got); err != nil {
				t.Fatal(err)
			}
		}
	}

	if got == nil {
		t.Fatalf("no SPEC message in output:\n%s", out)
	}

	data, err := os.ReadFile("spec.json")
	if err != nil {
		t.Fatal(err)
	}

	var want any
	if err := json.Unmarshal(data, &want); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("image spec differs from spec.json; run: task spec CONNECTOR=source-example")
	}
}
