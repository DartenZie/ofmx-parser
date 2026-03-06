package transform

import (
	"context"
	"testing"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

func TestDedupeAirspaceBordersSharedEdgeRenderedOnce(t *testing.T) {
	t.Parallel()

	borders := []domain.OFMXAirspaceBorder{
		{
			AirspaceID: "A",
			Points: []domain.OFMXGeoPoint{
				{Lat: 0, Lon: 0},
				{Lat: 0, Lon: 1},
				{Lat: 1, Lon: 1},
				{Lat: 1, Lon: 0},
			},
		},
		{
			AirspaceID: "B",
			Points: []domain.OFMXGeoPoint{
				{Lat: 0, Lon: 1},
				{Lat: 0, Lon: 2},
				{Lat: 1, Lon: 2},
				{Lat: 1, Lon: 1},
			},
		},
	}

	got := dedupeAirspaceBorders(borders)
	if len(got) != 7 {
		t.Fatalf("expected 7 unique border lines, got %d", len(got))
	}

	sharedCount := 0
	for _, edge := range got {
		if edge.Shared {
			sharedCount++
			if edge.ZoneA != "A" || edge.ZoneB != "B" {
				t.Fatalf("expected shared edge zone labels A/B, got %q/%q", edge.ZoneA, edge.ZoneB)
			}
		}
	}

	if sharedCount != 1 {
		t.Fatalf("expected exactly one shared edge, got %d", sharedCount)
	}
}

func TestDedupeAirspaceBordersUsesQuantizedCoordinates(t *testing.T) {
	t.Parallel()

	borders := []domain.OFMXAirspaceBorder{
		{
			AirspaceID: "A",
			Points: []domain.OFMXGeoPoint{
				{Lat: 49.0, Lon: 14.0},
				{Lat: 49.2, Lon: 14.1},
			},
		},
		{
			AirspaceID: "B",
			Points: []domain.OFMXGeoPoint{
				{Lat: 49.2000004, Lon: 14.1000004},
				{Lat: 49.0000004, Lon: 14.0000004},
			},
		},
	}

	got := dedupeAirspaceBorders(borders)
	if len(got) != 1 {
		t.Fatalf("expected quantized shared segment to dedupe to one edge, got %d", len(got))
	}

	if !got[0].Shared {
		t.Fatalf("expected deduped edge to be shared, got %+v", got[0])
	}
}

func TestDefaultMapMapperMapsZonesAndBorders(t *testing.T) {
	t.Parallel()

	input := domain.OFMXDocument{
		VORs:      []domain.OFMXVOR{{ID: "VLM", Name: "VLM", Lat: 50.1, Lon: 14.5}},
		Obstacles: []domain.OFMXObstacle{{ID: "OBS001", Name: "Mast", Lat: 49.3, Lon: 14.4}},
		Airspaces: []domain.OFMXAirspace{
			{ID: "A", Name: "Zone A", Type: "CTR", Class: "C", LowerRef: "SFC", UpperRef: "MSL", LowerValueM: 0, UpperValueM: 1000},
			{ID: "B", Name: "Zone B", Type: "TMA", Class: "C", LowerRef: "SFC", UpperRef: "MSL", LowerValueM: 0, UpperValueM: 1200},
		},
		AirspaceBorders: []domain.OFMXAirspaceBorder{
			{
				AirspaceID: "A",
				Points:     []domain.OFMXGeoPoint{{Lat: 0, Lon: 0}, {Lat: 0, Lon: 1}, {Lat: 1, Lon: 1}, {Lat: 1, Lon: 0}},
			},
			{
				AirspaceID: "B",
				Points:     []domain.OFMXGeoPoint{{Lat: 0, Lon: 1}, {Lat: 0, Lon: 2}, {Lat: 1, Lon: 2}, {Lat: 1, Lon: 1}},
			},
		},
	}

	got, err := DefaultMapMapper{}.MapToMapDataset(context.Background(), input)
	if err != nil {
		t.Fatalf("map to map dataset failed: %v", err)
	}

	if len(got.Zones) != 2 {
		t.Fatalf("expected 2 zones, got %d", len(got.Zones))
	}

	if len(got.Zones[0].Polygon) != 4 {
		t.Fatalf("expected first zone polygon to have 4 points, got %d", len(got.Zones[0].Polygon))
	}

	if len(got.AirspaceBorders) != 7 {
		t.Fatalf("expected 7 deduped borders, got %d", len(got.AirspaceBorders))
	}

	if len(got.PointsOfInterest) != 2 {
		t.Fatalf("expected 2 points of interest, got %d", len(got.PointsOfInterest))
	}
}

func TestDefaultMapMapperFiltersDisallowedAirspaceTypes(t *testing.T) {
	t.Parallel()

	input := domain.OFMXDocument{
		Airspaces: []domain.OFMXAirspace{
			{ID: "CTR1", Name: "Zone CTR", Type: "CTR", Class: "C", LowerRef: "SFC", UpperRef: "MSL", LowerValueM: 0, UpperValueM: 1000},
			{ID: "FIR1", Name: "Zone FIR", Type: "FIR", Class: "FIR", LowerRef: "SFC", UpperRef: "MSL", LowerValueM: 0, UpperValueM: 1000},
		},
		AirspaceBorders: []domain.OFMXAirspaceBorder{
			{AirspaceID: "CTR1", Points: []domain.OFMXGeoPoint{{Lat: 0, Lon: 0}, {Lat: 0, Lon: 1}, {Lat: 1, Lon: 1}, {Lat: 1, Lon: 0}}},
			{AirspaceID: "FIR1", Points: []domain.OFMXGeoPoint{{Lat: 5, Lon: 5}, {Lat: 5, Lon: 6}, {Lat: 6, Lon: 6}, {Lat: 6, Lon: 5}}},
		},
	}

	got, err := DefaultMapMapper{}.MapToMapDataset(context.Background(), input)
	if err != nil {
		t.Fatalf("map to map dataset failed: %v", err)
	}

	if len(got.Zones) != 1 || got.Zones[0].ID != "CTR1" {
		t.Fatalf("expected only allowed airspace zone, got %+v", got.Zones)
	}

	if len(got.AirspaceBorders) != 4 {
		t.Fatalf("expected borders only from allowed zone, got %d", len(got.AirspaceBorders))
	}
}

func TestDefaultMapMapperUsesConfiguredAirspaceTypeAllowlist(t *testing.T) {
	t.Parallel()

	input := domain.OFMXDocument{
		Airspaces: []domain.OFMXAirspace{
			{ID: "CTR1", Name: "Zone CTR", Type: "CTR", Class: "C", LowerRef: "SFC", UpperRef: "MSL", LowerValueM: 0, UpperValueM: 1000},
			{ID: "TMA1", Name: "Zone TMA", Type: "TMA", Class: "C", LowerRef: "SFC", UpperRef: "MSL", LowerValueM: 0, UpperValueM: 1000},
		},
		AirspaceBorders: []domain.OFMXAirspaceBorder{
			{AirspaceID: "CTR1", Points: []domain.OFMXGeoPoint{{Lat: 0, Lon: 0}, {Lat: 0, Lon: 1}, {Lat: 1, Lon: 1}, {Lat: 1, Lon: 0}}},
			{AirspaceID: "TMA1", Points: []domain.OFMXGeoPoint{{Lat: 5, Lon: 5}, {Lat: 5, Lon: 6}, {Lat: 6, Lon: 6}, {Lat: 6, Lon: 5}}},
		},
	}

	got, err := DefaultMapMapper{AllowedAirspaceTypes: []string{"TMA"}}.MapToMapDataset(context.Background(), input)
	if err != nil {
		t.Fatalf("map to map dataset failed: %v", err)
	}

	if len(got.Zones) != 1 || got.Zones[0].ID != "TMA1" {
		t.Fatalf("expected only TMA zone after configured filtering, got %+v", got.Zones)
	}

	if len(got.AirspaceBorders) != 4 {
		t.Fatalf("expected only one zone ring worth of borders, got %d", len(got.AirspaceBorders))
	}
}

func TestDefaultMapMapperFiltersByConfiguredMaxAltitudeFL(t *testing.T) {
	t.Parallel()

	input := domain.OFMXDocument{
		Airspaces: []domain.OFMXAirspace{
			{ID: "SFC1", Name: "Surface", Type: "CTR", LowerRef: "SFC", LowerValueM: 0, UpperRef: "MSL", UpperValueM: 1000},
			{ID: "HEI1", Name: "Height", Type: "CTR", LowerRef: "HEI", LowerValueM: 1200, UpperRef: "MSL", UpperValueM: 2500},
			{ID: "FL80", Name: "Low FL", Type: "TMA", LowerRef: "STD", LowerValueM: 80, UpperRef: "MSL", UpperValueM: 2000},
			{ID: "FL100", Name: "High FL", Type: "TMA", LowerRef: "STD", LowerValueM: 100, UpperRef: "MSL", UpperValueM: 3000},
			{ID: "ALT9400", Name: "ALT", Type: "R", LowerRef: "ALT", LowerValueM: 9400, UpperRef: "MSL", UpperValueM: 12000},
		},
		AirspaceBorders: []domain.OFMXAirspaceBorder{
			{AirspaceID: "SFC1", Points: []domain.OFMXGeoPoint{{Lat: 0, Lon: 0}, {Lat: 0, Lon: 1}, {Lat: 1, Lon: 1}, {Lat: 1, Lon: 0}}},
			{AirspaceID: "HEI1", Points: []domain.OFMXGeoPoint{{Lat: 1.5, Lon: 1.5}, {Lat: 1.5, Lon: 2.5}, {Lat: 2.5, Lon: 2.5}, {Lat: 2.5, Lon: 1.5}}},
			{AirspaceID: "FL80", Points: []domain.OFMXGeoPoint{{Lat: 2, Lon: 2}, {Lat: 2, Lon: 3}, {Lat: 3, Lon: 3}, {Lat: 3, Lon: 2}}},
			{AirspaceID: "FL100", Points: []domain.OFMXGeoPoint{{Lat: 4, Lon: 4}, {Lat: 4, Lon: 5}, {Lat: 5, Lon: 5}, {Lat: 5, Lon: 4}}},
			{AirspaceID: "ALT9400", Points: []domain.OFMXGeoPoint{{Lat: 6, Lon: 6}, {Lat: 6, Lon: 7}, {Lat: 7, Lon: 7}, {Lat: 7, Lon: 6}}},
		},
	}

	got, err := DefaultMapMapper{MaxAirspaceLowerFL: 95}.MapToMapDataset(context.Background(), input)
	if err != nil {
		t.Fatalf("map to map dataset failed: %v", err)
	}

	if len(got.Zones) != 4 {
		t.Fatalf("expected 4 zones below-or-equal FL95 threshold, got %+v", got.Zones)
	}

	zoneIDs := make(map[string]struct{}, len(got.Zones))
	for _, zone := range got.Zones {
		zoneIDs[zone.ID] = struct{}{}
	}
	if _, ok := zoneIDs["FL100"]; ok {
		t.Fatalf("expected FL100 zone to be filtered out, got %+v", got.Zones)
	}
	if _, ok := zoneIDs["ALT9400"]; !ok {
		t.Fatalf("expected ALT9400 zone to be included, got %+v", got.Zones)
	}
	if _, ok := zoneIDs["HEI1"]; !ok {
		t.Fatalf("expected HEI1 zone to be included as above-surface, got %+v", got.Zones)
	}
}
