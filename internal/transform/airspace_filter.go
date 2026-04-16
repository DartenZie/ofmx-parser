package transform

import (
	"math"
	"strings"

	"github.com/DartenZie/ofmx-parser/internal/domain"
)

const defaultMaxAirspaceLowerFL = 95

func effectiveMaxAirspaceLowerFL(custom int) int {
	if custom < defaultMaxAirspaceLowerFL {
		return defaultMaxAirspaceLowerFL
	}
	return custom
}

func passesAirspaceFilters(as domain.OFMXAirspace, allowedTypes map[string]struct{}, maxLowerFL int) bool {
	if !isAllowedAirspaceType(as.Type, allowedTypes) {
		return false
	}
	return airspaceLowerLimitFL(as) < float64(maxLowerFL)
}

func airspaceLowerLimitFL(as domain.OFMXAirspace) float64 {
	ref := strings.ToUpper(strings.TrimSpace(as.LowerRef))
	switch {
	case strings.Contains(ref, "SFC"), strings.Contains(ref, "AGL"), strings.Contains(ref, "HEI"):
		return 0
	case strings.Contains(ref, "MSL"), strings.Contains(ref, "AMSL"), strings.Contains(ref, "ALT"), strings.Contains(ref, "FT"):
		return math.Floor(as.LowerValueM / 100.0)
	case strings.Contains(ref, "STD"), strings.Contains(ref, "FL"):
		return as.LowerValueM
	default:
		return as.LowerValueM
	}
}
