package output

import (
	"context"
	"testing"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

func TestSemanticSchemaValidatorValidateAcceptsValidDocument(t *testing.T) {
	t.Parallel()

	doc := domain.OutputDocument{
		Cycle:       "20260115",
		Region:      "CZ",
		GeneratedAt: "2026-01-15T00:00:00Z",
		Schema:      "output.xsd",
		Airports: &domain.OutputAirports{Airports: []domain.OutputAirport{{
			ID:    "LKPR",
			D:     "AD",
			N:     "Prague Airport",
			Lat:   50.1,
			Lon:   14.2,
			ElevM: 380,
			Runways: &domain.OutputRunways{Runways: []domain.OutputRunway{{
				N:    "06/24",
				LenM: 3715,
				WidM: 45,
				Dirs: []domain.OutputRunwayDirection{{
					Bearing: 58,
					Code:    "ENE",
				}},
			}}},
		}}},
		Navaids: &domain.OutputNavaids{Navaids: []domain.OutputNavaid{{
			ID:  "VLM",
			T:   "VOR",
			D:   "VOR",
			N:   "VLM VOR",
			Lat: 50.2,
			Lon: 14.3,
		}}},
	}

	err := SemanticSchemaValidator{}.Validate(context.Background(), doc)
	if err != nil {
		t.Fatalf("expected valid doc, got error: %v", err)
	}
}

func TestSemanticSchemaValidatorValidateRejectsInvalidNavaidType(t *testing.T) {
	t.Parallel()

	doc := domain.OutputDocument{
		Cycle:       "20260115",
		Region:      "CZ",
		GeneratedAt: "2026-01-15T00:00:00Z",
		Schema:      "output.xsd",
		Navaids: &domain.OutputNavaids{Navaids: []domain.OutputNavaid{{
			ID:  "X1",
			T:   "ILS",
			D:   "ILS",
			N:   "ILS",
			Lat: 50,
			Lon: 14,
		}}},
	}

	err := SemanticSchemaValidator{}.Validate(context.Background(), doc)
	if err == nil {
		t.Fatal("expected validation error for invalid navaid type")
	}
}

func TestSemanticSchemaValidatorValidateRejectsInvalidRunwayBearing(t *testing.T) {
	t.Parallel()

	doc := domain.OutputDocument{
		Cycle:       "20260115",
		Region:      "CZ",
		GeneratedAt: "2026-01-15T00:00:00Z",
		Schema:      "output.xsd",
		Airports: &domain.OutputAirports{Airports: []domain.OutputAirport{{
			ID:    "LKPR",
			D:     "AD",
			N:     "Prague Airport",
			Lat:   50.1,
			Lon:   14.2,
			ElevM: 380,
			Runways: &domain.OutputRunways{Runways: []domain.OutputRunway{{
				N:    "06/24",
				LenM: 3715,
				WidM: 45,
				Dirs: []domain.OutputRunwayDirection{{
					Bearing: 400,
					Code:    "ENE",
				}},
			}}},
		}}},
	}

	err := SemanticSchemaValidator{}.Validate(context.Background(), doc)
	if err == nil {
		t.Fatal("expected validation error for invalid runway bearing")
	}
}

func TestSemanticSchemaValidatorValidateRejectsAirspaceWithTooFewPoints(t *testing.T) {
	t.Parallel()

	doc := domain.OutputDocument{
		Cycle:       "20260115",
		Region:      "CZ",
		GeneratedAt: "2026-01-15T00:00:00Z",
		Schema:      "output.xsd",
		Airspaces: &domain.OutputAirspaces{Airspaces: []domain.OutputAirspace{{
			ID:     "LKR1",
			D:      "C",
			N:      "Restricted",
			T:      "R",
			LowM:   0,
			LowRef: "AGL",
			UpM:    1000,
			UpRef:  "MSL",
			Poly: domain.OutputPolygon{Points: []domain.OutputPoint{
				{Lat: 49, Lon: 14},
				{Lat: 49.1, Lon: 14.1},
			}},
			BBox: domain.OutputBBox{MinLat: 49, MinLon: 14, MaxLat: 49.1, MaxLon: 14.1},
		}}},
	}

	err := SemanticSchemaValidator{}.Validate(context.Background(), doc)
	if err == nil {
		t.Fatal("expected validation error for polygon with fewer than 3 points")
	}
}
