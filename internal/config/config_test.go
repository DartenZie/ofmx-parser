package config

import "testing"

func TestParseArgsAcceptsMapOnlyMode(t *testing.T) {
	t.Parallel()

	_, err := ParseArgs([]string{
		"--input", "input.ofmx",
		"--pbf-input", "base.osm.pbf",
		"--pmtiles-output", "map.pmtiles",
	})
	if err != nil {
		t.Fatalf("expected valid map-only args, got: %v", err)
	}
}

func TestParseArgsRejectsMapModeWithoutPBF(t *testing.T) {
	t.Parallel()

	_, err := ParseArgs([]string{
		"--input", "input.ofmx",
		"--pmtiles-output", "map.pmtiles",
	})
	if err == nil {
		t.Fatal("expected error when --pbf-input missing in map mode")
	}
}

func TestParseArgsRejectsWhenNoOutputRequested(t *testing.T) {
	t.Parallel()

	_, err := ParseArgs([]string{"--input", "input.ofmx"})
	if err == nil {
		t.Fatal("expected error when no --output or --pmtiles-output provided")
	}
}

func TestParseArgsRejectsNonPositiveArcChord(t *testing.T) {
	t.Parallel()

	_, err := ParseArgs([]string{
		"--input", "input.ofmx",
		"--output", "out.xml",
		"--arc-max-chord-m", "0",
	})
	if err == nil {
		t.Fatal("expected error for non-positive --arc-max-chord-m")
	}
}
