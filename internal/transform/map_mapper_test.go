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
			{ID: "A", Name: "Zone A", Type: "R", Class: "C", LowerRef: "SFC", UpperRef: "MSL", LowerValueM: 0, UpperValueM: 1000},
			{ID: "B", Name: "Zone B", Type: "R", Class: "C", LowerRef: "SFC", UpperRef: "MSL", LowerValueM: 0, UpperValueM: 1200},
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
