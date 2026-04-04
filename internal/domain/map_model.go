// Package domain defines canonical OFMX input and output XML models.
//
// Author: Miroslav Pašek
package domain

// MapExportRequest defines map export inputs independent of CLI flags.
type MapExportRequest struct {
	PBFInputPath      string
	PMTilesOutputPath string
	GeoJSONOutputDir  string
	TilemakerBin      string
	TilemakerConfig   string
	TilemakerProcess  string
	TempDir           string
}

// MapGeoJSONArtifacts contains produced GeoJSON source file paths.
type MapGeoJSONArtifacts struct {
	AirportsPath          string
	ZonesPath             string
	PointsOfInterestPath  string
	AirspaceBordersPath   string
	CountriesBoundaryPath string
}

// MapDataset is a map-oriented intermediate model for PMTiles export.
type MapDataset struct {
	Airports         []MapAirportPoint
	Zones            []MapZonePolygon
	PointsOfInterest []MapPOI
	AirspaceBorders  []MapBorderLine
	CountryBorders   []MapCountryBoundary
}

// MapAirportPoint is the map representation of one airport.
type MapAirportPoint struct {
	ID    string
	Name  string
	Type  string
	Lat   float64
	Lon   float64
	ElevM float64
}

// MapZonePolygon is the map representation of one airspace zone.
type MapZonePolygon struct {
	ID      string
	Name    string
	Type    string
	Class   string
	LowM    float64
	LowRef  string
	UpM     float64
	UpRef   string
	Rmk     string
	Polygon []OFMXGeoPoint
}

// MapPOI is the map representation of one point of interest.
type MapPOI struct {
	ID   string
	Kind string
	Name string
	Type string
	Lat  float64
	Lon  float64
}

// MapBorderLine is one rendered airspace border segment.
type MapBorderLine struct {
	EdgeID   string
	ZoneA    string
	ZoneB    string
	ZoneType string
	Shared   bool
	Line     []OFMXGeoPoint
}

// MapCountryBoundary is one rendered country boundary polyline.
type MapCountryBoundary struct {
	UID  string
	Name string
	Line []OFMXGeoPoint
}
