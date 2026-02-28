package transform

import (
	"context"
	"testing"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

func TestDefaultMapperMapsAirportsRunwaysAndNavaids(t *testing.T) {
	t.Parallel()

	input := domain.OFMXDocument{
		SnapshotMeta: domain.OFMXSnapshotMetadata{
			Origin:    "unit-test",
			Regions:   "CZ",
			Created:   "2026-01-01T00:00:00Z",
			Effective: "2026-01-15T00:00:00Z",
		},
		Airports: []domain.OFMXAirport{{
			ID:    "LKPR",
			Name:  "Prague Airport",
			Type:  "AD",
			Lat:   50.100556,
			Lon:   14.262222,
			ElevM: 380,
		}},
		Runways: []domain.OFMXRunway{{
			AirportID:   "LKPR",
			Designation: "06/24",
			LengthM:     3715,
			WidthM:      45,
		}},
		RunwayDirections: []domain.OFMXRunwayDirection{{
			AirportID:    "LKPR",
			RunwayDesign: "06/24",
			Designator:   "06",
			TrueBearing:  58,
		}},
		VORs: []domain.OFMXVOR{{
			ID:   "VLM",
			Name: "VLM VOR",
			Type: "VOR",
			Lat:  50.116667,
			Lon:  14.5,
		}},
	}

	got, err := DefaultMapper{}.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("map failed: %v", err)
	}

	if got.Cycle != "20260115" {
		t.Fatalf("expected cycle 20260115, got %q", got.Cycle)
	}

	if got.Airports == nil || len(got.Airports.Airports) != 1 {
		t.Fatalf("expected 1 airport, got %+v", got.Airports)
	}

	airport := got.Airports.Airports[0]
	if airport.ID != "LKPR" || airport.N != "Prague Airport" {
		t.Fatalf("unexpected airport mapping: %+v", airport)
	}

	if airport.Runways == nil || len(airport.Runways.Runways) != 1 {
		t.Fatalf("expected 1 runway, got %+v", airport.Runways)
	}

	rwy := airport.Runways.Runways[0]
	if len(rwy.Dirs) != 1 || rwy.Dirs[0].Code != "ENE" {
		t.Fatalf("expected mapped runway direction ENE from 58 deg, got %+v", rwy.Dirs)
	}

	if got.Navaids == nil || len(got.Navaids.Navaids) != 1 {
		t.Fatalf("expected 1 navaid, got %+v", got.Navaids)
	}

	if got.Navaids.Navaids[0].T != "VOR" {
		t.Fatalf("expected VOR navaid type, got %+v", got.Navaids.Navaids[0])
	}
}

func TestDefaultMapperMapsAirspacesAndObstacles(t *testing.T) {
	t.Parallel()

	input := domain.OFMXDocument{
		SnapshotMeta: domain.OFMXSnapshotMetadata{
			Origin:    "unit-test",
			Regions:   "CZ",
			Created:   "2026-01-01T00:00:00Z",
			Effective: "2026-01-15T00:00:00Z",
		},
		Airspaces: []domain.OFMXAirspace{{
			ID:          "LKR1",
			Type:        "R",
			Name:        "Restricted Area",
			Class:       "C",
			LowerValueM: 0,
			LowerRef:    "SFC",
			UpperValueM: 2450,
			UpperRef:    "MSL",
			Remark:      "TEST",
		}},
		AirspaceBorders: []domain.OFMXAirspaceBorder{{
			AirspaceID: "LKR1",
			Points: []domain.OFMXGeoPoint{
				{Lat: 49.0, Lon: 14.0},
				{Lat: 49.1, Lon: 14.2},
				{Lat: 48.9, Lon: 14.3},
			},
		}},
		Obstacles: []domain.OFMXObstacle{{
			ID:         "OBS001",
			Type:       "TOWER",
			Name:       "Mast",
			Lat:        49.3,
			Lon:        14.4,
			HeightM:    120,
			ElevationM: 300,
		}},
	}

	got, err := DefaultMapper{}.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("map failed: %v", err)
	}

	if got.Airspaces == nil || len(got.Airspaces.Airspaces) != 1 {
		t.Fatalf("expected 1 airspace, got %+v", got.Airspaces)
	}

	as := got.Airspaces.Airspaces[0]
	if as.ID != "LKR1" || as.LowRef != "AGL" || as.UpRef != "MSL" {
		t.Fatalf("unexpected airspace mapping: %+v", as)
	}

	if len(as.Poly.Points) < 3 {
		t.Fatalf("expected >=3 polygon points, got %+v", as.Poly.Points)
	}

	if got.Obstacles == nil || len(got.Obstacles.Obstacles) != 1 {
		t.Fatalf("expected 1 obstacle, got %+v", got.Obstacles)
	}

	obs := got.Obstacles.Obstacles[0]
	if obs.ID != "OBS001" || obs.HM != 120 {
		t.Fatalf("unexpected obstacle mapping: %+v", obs)
	}
}
