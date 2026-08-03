package resource

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDropInFileResource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "limits.conf")
	r := &DropInFileResource{ResourceName: "limits", Path: path, Content: "* soft nofile 1048576\n"}

	res, err := r.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusWouldChange {
		t.Fatalf("expected would_change for missing file, got %s", res.Status)
	}

	if err := r.Apply(context.Background()); err != nil {
		t.Fatalf("apply: %v", err)
	}

	res, err = r.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusUnchanged {
		t.Fatalf("expected unchanged after apply, got %s", res.Status)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != r.Content {
		t.Fatalf("unexpected content: %q", data)
	}
}
