package integration

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/DartenZie/ofmx-parser/internal/app"
)

func TestAppRunFailsWhenOutputViolatesSchemaRules(t *testing.T) {
	t.Parallel()

	input := `<?xml version="1.0" encoding="UTF-8"?>
<OFMX-Snapshot version="0.1.0" origin="unit-test" namespace="123e4567-e89b-12d3-a456-426614174000" created="not-a-datetime" effective="2026-01-01T00:00:00Z">
  <Ahp>
    <AhpUid><codeId>LKTB</codeId></AhpUid>
    <OrgUid><txtName>Authority</txtName></OrgUid>
    <txtName>Brno Airport</txtName>
    <codeType>AD</codeType>
    <geoLat>49.150000N</geoLat>
    <geoLong>016.683333E</geoLong>
    <valElev>235</valElev>
  </Ahp>
</OFMX-Snapshot>`

	err := app.Run(context.Background(), []string{"--input", writeTempInput(t, input), "--output", tempOutputPath(t)})
	if err == nil {
		t.Fatal("expected schema validation error")
	}

	if !strings.Contains(err.Error(), "E_VALIDATE") {
		t.Fatalf("expected E_VALIDATE error, got %v", err)
	}
}

func writeTempInput(t *testing.T, content string) string {
	t.Helper()

	tmpDir := t.TempDir()
	path := tmpDir + "/input.ofmx"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write temp input: %v", err)
	}

	return path
}

func tempOutputPath(t *testing.T) string {
	t.Helper()
	return t.TempDir() + "/output.xml"
}
