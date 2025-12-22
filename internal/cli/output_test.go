package cli

import (
	"bytes"
	"testing"
)

type sampleOutput struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func TestFormatterJSON(t *testing.T) {
	t.Cleanup(func() {
		jsonOutput = false
		jsonlOutput = false
	})

	jsonOutput = true
	jsonlOutput = false

	var buf bytes.Buffer
	formatter := NewFormatter(&buf)
	if err := formatter.Write(sampleOutput{Name: "alpha", Count: 2}); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	expected := "{\n  \"name\": \"alpha\",\n  \"count\": 2\n}\n"
	if buf.String() != expected {
		t.Fatalf("unexpected JSON output:\n%s", buf.String())
	}
}

func TestFormatterJSONL(t *testing.T) {
	t.Cleanup(func() {
		jsonOutput = false
		jsonlOutput = false
	})

	jsonOutput = false
	jsonlOutput = true

	var buf bytes.Buffer
	formatter := NewFormatter(&buf)
	payload := []sampleOutput{
		{Name: "alpha", Count: 1},
		{Name: "beta", Count: 2},
	}
	if err := formatter.Write(payload); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	expected := "{\"name\":\"alpha\",\"count\":1}\n{\"name\":\"beta\",\"count\":2}\n"
	if buf.String() != expected {
		t.Fatalf("unexpected JSONL output:\n%s", buf.String())
	}
}

func TestFormatterHuman(t *testing.T) {
	t.Cleanup(func() {
		jsonOutput = false
		jsonlOutput = false
	})

	jsonOutput = false
	jsonlOutput = false

	var buf bytes.Buffer
	formatter := NewFormatter(&buf)
	if err := formatter.Write("hello"); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if buf.String() != "hello\n" {
		t.Fatalf("unexpected human output: %q", buf.String())
	}
}
