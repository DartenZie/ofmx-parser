package integration

import (
	"os"
	"path/filepath"
	"testing"
)

func fixtureInput(t *testing.T, name string) string {
	t.Helper()

	path := filepath.Join("..", "fixtures", "input", name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %q: %v", name, err)
	}

	return string(b)
}
