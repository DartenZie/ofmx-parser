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
			Type:        "CTR",
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

func TestDefaultMapperFiltersDisallowedAirspaceTypes(t *testing.T) {
	t.Parallel()

	input := domain.OFMXDocument{
		Airspaces: []domain.OFMXAirspace{
			{ID: "LKCTR", Type: "CTR", Name: "Control Zone", LowerRef: "SFC", UpperRef: "MSL", UpperValueM: 1000},
			{ID: "LKFIRX", Type: "FIR", Name: "Flight Info Region", LowerRef: "SFC", UpperRef: "MSL", UpperValueM: 1000},
		},
		AirspaceBorders: []domain.OFMXAirspaceBorder{
			{AirspaceID: "LKCTR", Points: []domain.OFMXGeoPoint{{Lat: 49, Lon: 14}, {Lat: 49.1, Lon: 14.1}, {Lat: 49, Lon: 14.2}}},
			{AirspaceID: "LKFIRX", Points: []domain.OFMXGeoPoint{{Lat: 48, Lon: 13}, {Lat: 48.1, Lon: 13.1}, {Lat: 48, Lon: 13.2}}},
		},
	}

	got, err := DefaultMapper{}.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("map failed: %v", err)
	}

	if got.Airspaces == nil || len(got.Airspaces.Airspaces) != 1 {
		t.Fatalf("expected exactly one allowed airspace, got %+v", got.Airspaces)
	}

	if got.Airspaces.Airspaces[0].ID != "LKCTR" {
		t.Fatalf("expected only CTR airspace to be mapped, got %+v", got.Airspaces.Airspaces)
	}
}

func TestDefaultMapperUsesConfiguredAirspaceTypeAllowlist(t *testing.T) {
	t.Parallel()

	input := domain.OFMXDocument{
		Airspaces: []domain.OFMXAirspace{
			{ID: "CTR1", Type: "CTR", Name: "Control Zone", LowerRef: "SFC", UpperRef: "MSL", UpperValueM: 1000},
			{ID: "TMA1", Type: "TMA", Name: "Terminal Area", LowerRef: "SFC", UpperRef: "MSL", UpperValueM: 2000},
		},
		AirspaceBorders: []domain.OFMXAirspaceBorder{
			{AirspaceID: "CTR1", Points: []domain.OFMXGeoPoint{{Lat: 49, Lon: 14}, {Lat: 49.1, Lon: 14.1}, {Lat: 49, Lon: 14.2}}},
			{AirspaceID: "TMA1", Points: []domain.OFMXGeoPoint{{Lat: 48, Lon: 13}, {Lat: 48.1, Lon: 13.1}, {Lat: 48, Lon: 13.2}}},
		},
	}

	got, err := DefaultMapper{AllowedAirspaceTypes: []string{"TMA"}}.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("map failed: %v", err)
	}

	if got.Airspaces == nil || len(got.Airspaces.Airspaces) != 1 {
		t.Fatalf("expected one configured airspace type, got %+v", got.Airspaces)
	}
	if got.Airspaces.Airspaces[0].ID != "TMA1" {
		t.Fatalf("expected only TMA1 to remain, got %+v", got.Airspaces.Airspaces)
	}
}

func TestDefaultMapperFiltersByConfiguredMaxAltitudeFL(t *testing.T) {
	t.Parallel()

	input := domain.OFMXDocument{
		Airspaces: []domain.OFMXAirspace{
			{ID: "SFC1", Type: "CTR", Name: "Surface Zone", LowerRef: "SFC", LowerValueM: 0, UpperRef: "MSL", UpperValueM: 1000},
			{ID: "HEI1", Type: "CTR", Name: "Height Zone", LowerRef: "HEI", LowerValueM: 1500, UpperRef: "MSL", UpperValueM: 3000},
			{ID: "FL80", Type: "TMA", Name: "Low FL Zone", LowerRef: "STD", LowerValueM: 80, UpperRef: "MSL", UpperValueM: 2000},
			{ID: "FL100", Type: "TMA", Name: "High FL Zone", LowerRef: "STD", LowerValueM: 100, UpperRef: "MSL", UpperValueM: 3000},
			{ID: "MSL9500", Type: "R", Name: "MSL Zone", LowerRef: "MSL", LowerValueM: 9500, UpperRef: "MSL", UpperValueM: 12000},
			{ID: "ALT9400", Type: "R", Name: "ALT Zone", LowerRef: "ALT", LowerValueM: 9400, UpperRef: "MSL", UpperValueM: 12000},
			{ID: "FT9600", Type: "R", Name: "FT Zone", LowerRef: "FT", LowerValueM: 9600, UpperRef: "MSL", UpperValueM: 12000},
		},
		AirspaceBorders: []domain.OFMXAirspaceBorder{
			{AirspaceID: "SFC1", Points: []domain.OFMXGeoPoint{{Lat: 49, Lon: 14}, {Lat: 49.1, Lon: 14.1}, {Lat: 49, Lon: 14.2}}},
			{AirspaceID: "HEI1", Points: []domain.OFMXGeoPoint{{Lat: 49.2, Lon: 14.3}, {Lat: 49.3, Lon: 14.4}, {Lat: 49.2, Lon: 14.5}}},
			{AirspaceID: "FL80", Points: []domain.OFMXGeoPoint{{Lat: 48, Lon: 13}, {Lat: 48.1, Lon: 13.1}, {Lat: 48, Lon: 13.2}}},
			{AirspaceID: "FL100", Points: []domain.OFMXGeoPoint{{Lat: 47, Lon: 12}, {Lat: 47.1, Lon: 12.1}, {Lat: 47, Lon: 12.2}}},
			{AirspaceID: "MSL9500", Points: []domain.OFMXGeoPoint{{Lat: 46, Lon: 11}, {Lat: 46.1, Lon: 11.1}, {Lat: 46, Lon: 11.2}}},
			{AirspaceID: "ALT9400", Points: []domain.OFMXGeoPoint{{Lat: 45, Lon: 10}, {Lat: 45.1, Lon: 10.1}, {Lat: 45, Lon: 10.2}}},
			{AirspaceID: "FT9600", Points: []domain.OFMXGeoPoint{{Lat: 44, Lon: 9}, {Lat: 44.1, Lon: 9.1}, {Lat: 44, Lon: 9.2}}},
		},
	}

	got, err := DefaultMapper{MaxAirspaceLowerFL: 95}.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("map failed: %v", err)
	}

	if got.Airspaces == nil {
		t.Fatalf("expected filtered airspaces, got nil")
	}

	ids := make(map[string]struct{}, len(got.Airspaces.Airspaces))
	for _, as := range got.Airspaces.Airspaces {
		ids[as.ID] = struct{}{}
	}

	for _, id := range []string{"SFC1", "HEI1", "FL80", "MSL9500", "ALT9400"} {
		if _, ok := ids[id]; !ok {
			t.Fatalf("expected airspace %s to pass max-altitude filter, got %+v", id, got.Airspaces.Airspaces)
		}
	}
	for _, id := range []string{"FL100", "FT9600"} {
		if _, ok := ids[id]; ok {
			t.Fatalf("expected %s to be filtered out, got %+v", id, got.Airspaces.Airspaces)
		}
	}
}
