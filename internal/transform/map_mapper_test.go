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
	if len(got) != 3 {
		t.Fatalf("expected 3 merged border lines, got %d", len(got))
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

	hasLongOpenLine := false
	for _, edge := range got {
		if !edge.Shared && len(edge.Line) > 2 {
			hasLongOpenLine = true
			break
		}
	}
	if !hasLongOpenLine {
		t.Fatalf("expected non-shared borders to merge into longer lines, got %+v", got)
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
		CountryBorders: []domain.OFMXGeographicalBorder{
			{UID: "CB-2", Name: "CZ-PL", Points: []domain.OFMXGeoPoint{{Lat: 50.0, Lon: 15.0}, {Lat: 50.1, Lon: 15.1}}},
			{UID: "CB-1", Name: "CZ-DE", Points: []domain.OFMXGeoPoint{{Lat: 49.0, Lon: 14.0}, {Lat: 49.1, Lon: 14.1}}},
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

	if len(got.AirspaceBorders) != 4 {
		t.Fatalf("expected 4 merged borders with split shared edge by zone type, got %d", len(got.AirspaceBorders))
	}

	if len(got.PointsOfInterest) != 2 {
		t.Fatalf("expected 2 points of interest, got %d", len(got.PointsOfInterest))
	}

	seenTypes := map[string]bool{}
	sharedByType := map[string]int{}
	for _, border := range got.AirspaceBorders {
		if border.ZoneType != "" {
			seenTypes[border.ZoneType] = true
		}
		if border.Shared {
			sharedByType[border.ZoneType]++
		}
	}
	if !seenTypes["CTR"] || !seenTypes["TMA"] {
		t.Fatalf("expected zone types CTR and TMA on borders, got %+v", got.AirspaceBorders)
	}
	if sharedByType["CTR"] != 1 || sharedByType["TMA"] != 1 {
		t.Fatalf("expected one shared border for each zone type, got %+v", sharedByType)
	}

	if len(got.CountryBorders) != 2 {
		t.Fatalf("expected 2 country borders, got %d", len(got.CountryBorders))
	}
	if got.CountryBorders[0].UID != "CB-1" {
		t.Fatalf("expected country boundaries sorted by UID, got %+v", got.CountryBorders)
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

	if len(got.AirspaceBorders) != 1 {
		t.Fatalf("expected one merged border line from allowed zone, got %d", len(got.AirspaceBorders))
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

	if len(got.AirspaceBorders) != 1 {
		t.Fatalf("expected one merged border line, got %d", len(got.AirspaceBorders))
	}
}

func TestDefaultMapMapperKeepsSharedBorderCollapsedForSameZoneType(t *testing.T) {
	t.Parallel()

	input := domain.OFMXDocument{
		Airspaces: []domain.OFMXAirspace{
			{ID: "A", Name: "Zone A", Type: "CTR", Class: "C", LowerRef: "SFC", UpperRef: "MSL", LowerValueM: 0, UpperValueM: 1000},
			{ID: "B", Name: "Zone B", Type: "CTR", Class: "C", LowerRef: "SFC", UpperRef: "MSL", LowerValueM: 0, UpperValueM: 1200},
		},
		AirspaceBorders: []domain.OFMXAirspaceBorder{
			{AirspaceID: "A", Points: []domain.OFMXGeoPoint{{Lat: 0, Lon: 0}, {Lat: 0, Lon: 1}, {Lat: 1, Lon: 1}, {Lat: 1, Lon: 0}}},
			{AirspaceID: "B", Points: []domain.OFMXGeoPoint{{Lat: 0, Lon: 1}, {Lat: 0, Lon: 2}, {Lat: 1, Lon: 2}, {Lat: 1, Lon: 1}}},
		},
	}

	got, err := DefaultMapMapper{}.MapToMapDataset(context.Background(), input)
	if err != nil {
		t.Fatalf("map to map dataset failed: %v", err)
	}

	if len(got.AirspaceBorders) != 3 {
		t.Fatalf("expected shared border to stay collapsed to one edge for same zone type, got %d", len(got.AirspaceBorders))
	}

	sharedCount := 0
	for _, border := range got.AirspaceBorders {
		if border.Shared {
			sharedCount++
			if border.ZoneType != "CTR" {
				t.Fatalf("expected shared border zone type CTR, got %+v", border)
			}
		}
	}

	if sharedCount != 1 {
		t.Fatalf("expected one shared border record for same zone type, got %d", sharedCount)
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

func TestDefaultMapMapperUsesLargestDisconnectedBorderForZonePolygon(t *testing.T) {
	t.Parallel()

	input := domain.OFMXDocument{
		Airspaces: []domain.OFMXAirspace{
			{ID: "ZONE1", Name: "Zone", Type: "CTR", Class: "C", LowerRef: "SFC", UpperRef: "MSL", LowerValueM: 0, UpperValueM: 1000},
		},
		AirspaceBorders: []domain.OFMXAirspaceBorder{
			{AirspaceID: "ZONE1", Points: []domain.OFMXGeoPoint{{Lat: 49.0, Lon: 14.0}, {Lat: 49.01, Lon: 14.01}, {Lat: 49.0, Lon: 14.02}}},
			{AirspaceID: "ZONE1", Points: []domain.OFMXGeoPoint{{Lat: 49.2, Lon: 14.2}, {Lat: 49.4, Lon: 14.3}, {Lat: 49.2, Lon: 14.5}}},
		},
	}

	got, err := DefaultMapMapper{}.MapToMapDataset(context.Background(), input)
	if err != nil {
		t.Fatalf("map to map dataset failed: %v", err)
	}

	if len(got.Zones) != 1 {
		t.Fatalf("expected one zone, got %+v", got.Zones)
	}

	if len(got.Zones[0].Polygon) != 3 {
		t.Fatalf("expected polygon from one disconnected border, got %d", len(got.Zones[0].Polygon))
	}

	if got.Zones[0].Polygon[0].Lat != 49.2 {
		t.Fatalf("expected larger disconnected border to be selected, got %+v", got.Zones[0].Polygon)
	}
}

func TestResolveZonePolygonStitchesConnectedParts(t *testing.T) {
	t.Parallel()

	parts := [][]domain.OFMXGeoPoint{
		{{Lat: 49.0, Lon: 14.0}, {Lat: 49.1, Lon: 14.1}, {Lat: 49.2, Lon: 14.2}},
		{{Lat: 49.2, Lon: 14.2}, {Lat: 49.3, Lon: 14.3}, {Lat: 49.4, Lon: 14.4}},
	}

	got := resolveZonePolygon(parts)
	if len(got) != 5 {
		t.Fatalf("expected connected chains to stitch into 5-point polygon path, got %d", len(got))
	}
}

func TestDefaultMapMapperMapsVocalicNavaidNameForPMTiles(t *testing.T) {
	t.Parallel()

	input := domain.OFMXDocument{
		VORs: []domain.OFMXVOR{
			{ID: "AB", Name: "AB", Lat: 50.1, Lon: 14.5},
			{ID: "NDB1", Name: "NDB1", Lat: 50.2, Lon: 14.6},
		},
	}

	got, err := DefaultMapMapper{}.MapToMapDataset(context.Background(), input)
	if err != nil {
		t.Fatalf("map to map dataset failed: %v", err)
	}

	if len(got.PointsOfInterest) != 2 {
		t.Fatalf("expected 2 points of interest, got %d", len(got.PointsOfInterest))
	}

	var vocalicPOI, plainPOI domain.MapPOI
	for _, poi := range got.PointsOfInterest {
		switch poi.ID {
		case "AB":
			vocalicPOI = poi
		case "NDB1":
			plainPOI = poi
		}
	}

	if vocalicPOI.Name != "Alpha Bravo" {
		t.Fatalf("expected vocalic navaid name Alpha Bravo, got %q", vocalicPOI.Name)
	}
	if vocalicPOI.Type != "vocalic" {
		t.Fatalf("expected vocalic navaid type, got %q", vocalicPOI.Type)
	}

	if plainPOI.Name != "NDB1" {
		t.Fatalf("expected non-vocalic navaid name unchanged, got %q", plainPOI.Name)
	}
	if plainPOI.Type != "" {
		t.Fatalf("expected non-vocalic navaid type empty, got %q", plainPOI.Type)
	}
}
