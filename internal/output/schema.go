// Package output validates and serializes custom XML output.
//
// Author: Miroslav Pašek
package output

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

type SchemaValidator interface {
	Validate(ctx context.Context, doc domain.OutputDocument) error
}

type NoopSchemaValidator struct{}

func (v NoopSchemaValidator) Validate(_ context.Context, _ domain.OutputDocument) error {
	return nil
}

type SemanticSchemaValidator struct{}

// Validate applies semantic validation rules for output XML constraints.
func (v SemanticSchemaValidator) Validate(_ context.Context, doc domain.OutputDocument) error {
	if strings.TrimSpace(doc.Cycle) == "" {
		return domain.NewError(domain.ErrValidate, "NavSnapshot/@cycle is required", nil)
	}
	if strings.TrimSpace(doc.Region) == "" {
		return domain.NewError(domain.ErrValidate, "NavSnapshot/@region is required", nil)
	}
	if strings.TrimSpace(doc.Schema) == "" {
		return domain.NewError(domain.ErrValidate, "NavSnapshot/@schema is required", nil)
	}
	if strings.TrimSpace(doc.GeneratedAt) == "" {
		return domain.NewError(domain.ErrValidate, "NavSnapshot/@generatedAt is required", nil)
	}
	if _, err := time.Parse(time.RFC3339, doc.GeneratedAt); err != nil {
		return domain.NewError(domain.ErrValidate, "NavSnapshot/@generatedAt must be RFC3339 datetime", err)
	}

	if doc.Airports != nil {
		for i, ap := range doc.Airports.Airports {
			if err := validateAirport(i, ap); err != nil {
				return err
			}
		}
	}

	if doc.Navaids != nil {
		for i, nv := range doc.Navaids.Navaids {
			if err := validateNavaid(i, nv); err != nil {
				return err
			}
		}
	}

	if doc.Airspaces != nil {
		for i, as := range doc.Airspaces.Airspaces {
			if err := validateAirspace(i, as); err != nil {
				return err
			}
		}
	}

	if doc.Obstacles != nil {
		for i, obs := range doc.Obstacles.Obstacles {
			if err := validateObstacle(i, obs); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateAirport(index int, ap domain.OutputAirport) error {
	prefix := fmt.Sprintf("Airports/Airport[%d]", index)

	if strings.TrimSpace(ap.ID) == "" {
		return domain.NewError(domain.ErrValidate, prefix+"/@id is required", nil)
	}
	if strings.TrimSpace(ap.D) == "" {
		return domain.NewError(domain.ErrValidate, prefix+"/@d is required", nil)
	}
	if strings.TrimSpace(ap.N) == "" {
		return domain.NewError(domain.ErrValidate, prefix+"/@n is required", nil)
	}
	if ap.Lat < -90 || ap.Lat > 90 {
		return domain.NewError(domain.ErrValidate, prefix+"/@lat out of range [-90,90]", nil)
	}
	if ap.Lon < -180 || ap.Lon > 180 {
		return domain.NewError(domain.ErrValidate, prefix+"/@lon out of range [-180,180]", nil)
	}

	if ap.Runways != nil {
		for i, rwy := range ap.Runways.Runways {
			if err := validateRunway(prefix, i, rwy); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateRunway(prefix string, index int, rwy domain.OutputRunway) error {
	path := fmt.Sprintf("%s/Runways/Runway[%d]", prefix, index)

	if strings.TrimSpace(rwy.N) == "" {
		return domain.NewError(domain.ErrValidate, path+"/@n is required", nil)
	}
	if rwy.LenM < 0 {
		return domain.NewError(domain.ErrValidate, path+"/@lenM must be >= 0", nil)
	}
	if rwy.WidM < 0 {
		return domain.NewError(domain.ErrValidate, path+"/@widM must be >= 0", nil)
	}
	if len(rwy.Dirs) == 0 {
		return domain.NewError(domain.ErrValidate, path+" requires at least one Dir element", nil)
	}

	for i, dir := range rwy.Dirs {
		dirPath := fmt.Sprintf("%s/Dir[%d]", path, i)
		if dir.Bearing < 0 || dir.Bearing > 360 {
			return domain.NewError(domain.ErrValidate, dirPath+"/@brg out of range [0,360]", nil)
		}
		if _, ok := validRunwayDirectionCode[dir.Code]; !ok {
			return domain.NewError(domain.ErrValidate, dirPath+"/@code must be one of RunwayDirectionCodeEnum", nil)
		}
	}

	return nil
}

func validateNavaid(index int, nv domain.OutputNavaid) error {
	path := fmt.Sprintf("Navaids/Navaid[%d]", index)

	if strings.TrimSpace(nv.ID) == "" {
		return domain.NewError(domain.ErrValidate, path+"/@id is required", nil)
	}
	if strings.TrimSpace(nv.D) == "" {
		return domain.NewError(domain.ErrValidate, path+"/@d is required", nil)
	}
	if strings.TrimSpace(nv.N) == "" {
		return domain.NewError(domain.ErrValidate, path+"/@n is required", nil)
	}
	if _, ok := validNavaidType[nv.T]; !ok {
		return domain.NewError(domain.ErrValidate, path+"/@t must be one of NavaidTypeEnum", nil)
	}
	if nv.Lat < -90 || nv.Lat > 90 {
		return domain.NewError(domain.ErrValidate, path+"/@lat out of range [-90,90]", nil)
	}
	if nv.Lon < -180 || nv.Lon > 180 {
		return domain.NewError(domain.ErrValidate, path+"/@lon out of range [-180,180]", nil)
	}

	return nil
}

func validateAirspace(index int, as domain.OutputAirspace) error {
	path := fmt.Sprintf("Airspaces/Airspace[%d]", index)

	if strings.TrimSpace(as.ID) == "" {
		return domain.NewError(domain.ErrValidate, path+"/@id is required", nil)
	}
	if strings.TrimSpace(as.D) == "" {
		return domain.NewError(domain.ErrValidate, path+"/@d is required", nil)
	}
	if strings.TrimSpace(as.N) == "" {
		return domain.NewError(domain.ErrValidate, path+"/@n is required", nil)
	}
	if strings.TrimSpace(as.T) == "" {
		return domain.NewError(domain.ErrValidate, path+"/@t is required", nil)
	}
	if _, ok := validHeightRef[as.LowRef]; !ok {
		return domain.NewError(domain.ErrValidate, path+"/@lowRef must be one of HeightRefEnum", nil)
	}
	if _, ok := validHeightRef[as.UpRef]; !ok {
		return domain.NewError(domain.ErrValidate, path+"/@upRef must be one of HeightRefEnum", nil)
	}
	if len(as.Poly.Points) < 3 {
		return domain.NewError(domain.ErrValidate, path+"/Poly must contain at least 3 points", nil)
	}

	for i, p := range as.Poly.Points {
		pp := fmt.Sprintf("%s/Poly/P[%d]", path, i)
		if p.Lat < -90 || p.Lat > 90 {
			return domain.NewError(domain.ErrValidate, pp+"/@lat out of range [-90,90]", nil)
		}
		if p.Lon < -180 || p.Lon > 180 {
			return domain.NewError(domain.ErrValidate, pp+"/@lon out of range [-180,180]", nil)
		}
	}

	if as.BBox.MinLat < -90 || as.BBox.MinLat > 90 || as.BBox.MaxLat < -90 || as.BBox.MaxLat > 90 {
		return domain.NewError(domain.ErrValidate, path+"/BBox latitude values out of range", nil)
	}
	if as.BBox.MinLon < -180 || as.BBox.MinLon > 180 || as.BBox.MaxLon < -180 || as.BBox.MaxLon > 180 {
		return domain.NewError(domain.ErrValidate, path+"/BBox longitude values out of range", nil)
	}

	return nil
}

func validateObstacle(index int, obs domain.OutputObstacle) error {
	path := fmt.Sprintf("Obstacles/Obstacle[%d]", index)

	if strings.TrimSpace(obs.ID) == "" {
		return domain.NewError(domain.ErrValidate, path+"/@id is required", nil)
	}
	if strings.TrimSpace(obs.T) == "" {
		return domain.NewError(domain.ErrValidate, path+"/@t is required", nil)
	}
	if strings.TrimSpace(obs.N) == "" {
		return domain.NewError(domain.ErrValidate, path+"/@n is required", nil)
	}
	if obs.Lat < -90 || obs.Lat > 90 {
		return domain.NewError(domain.ErrValidate, path+"/@lat out of range [-90,90]", nil)
	}
	if obs.Lon < -180 || obs.Lon > 180 {
		return domain.NewError(domain.ErrValidate, path+"/@lon out of range [-180,180]", nil)
	}
	if obs.HM < 0 {
		return domain.NewError(domain.ErrValidate, path+"/@hM must be >= 0", nil)
	}

	return nil
}

var validNavaidType = map[string]struct{}{
	"VOR":        {},
	"NDB":        {},
	"DME":        {},
	"DESIGNATED": {},
	"MARKER":     {},
	"TACAN":      {},
	"UNKNOWN":    {},
}

var validRunwayDirectionCode = map[string]struct{}{
	"N":   {},
	"NNE": {},
	"NE":  {},
	"ENE": {},
	"E":   {},
	"ESE": {},
	"SE":  {},
	"SSE": {},
	"S":   {},
	"SSW": {},
	"SW":  {},
	"WSW": {},
	"W":   {},
	"WNW": {},
	"NW":  {},
	"NNW": {},
}

var validHeightRef = map[string]struct{}{
	"AGL": {},
	"MSL": {},
	"FL":  {},
	"UNL": {},
	"STD": {},
}
