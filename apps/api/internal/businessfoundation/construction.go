package businessfoundation

import (
	"fmt"
	"math/big"
)

type ConstructionOperation string

const (
	ConstructionArea     ConstructionOperation = "area"
	ConstructionVolume   ConstructionOperation = "volume"
	ConstructionTiles    ConstructionOperation = "tiles"
	ConstructionBlocks   ConstructionOperation = "blocks"
	ConstructionPackages ConstructionOperation = "packages"

	maxConstructionLengthM = 10_000
	maxConstructionWaste   = 100
	maxConstructionCount   = 1_000_000_000
)

// ConstructionMeasure is a positive decimal (up to nine fractional digits)
// with an explicit metric unit: mm, cm, m, or their squared or cubed forms.
type ConstructionMeasure struct {
	Value string `json:"value"`
	Unit  string `json:"unit"`
}

// ConstructionInput describes a single rectangular estimate. Height is required
// for volume and selects cubic rather than square coverage for packages.
// Tiles and blocks require item face dimensions. Zero UnitsPerPackage means
// package rounding was not requested for those two operations.
type ConstructionInput struct {
	Operation       ConstructionOperation `json:"operation"`
	Length          ConstructionMeasure   `json:"length"`
	Width           ConstructionMeasure   `json:"width"`
	Height          ConstructionMeasure   `json:"height,omitzero"`
	ItemLength      ConstructionMeasure   `json:"item_length,omitzero"`
	ItemWidth       ConstructionMeasure   `json:"item_width,omitzero"`
	PackageCoverage ConstructionMeasure   `json:"package_coverage,omitzero"`
	UnitsPerPackage int64                 `json:"units_per_package,omitempty"`
	WastePercent    string                `json:"waste_percent"`
}

// ConstructionIntermediate contains exact decimal values in metres (dimensions)
// and BaseUnit (base, waste, adjusted measure and coverage per item or package).
// UnroundedCount is an exact rational string such as "10/3" or "5".
type ConstructionIntermediate struct {
	LengthM         string `json:"length_m"`
	WidthM          string `json:"width_m"`
	HeightM         string `json:"height_m,omitempty"`
	ItemLengthM     string `json:"item_length_m,omitempty"`
	ItemWidthM      string `json:"item_width_m,omitempty"`
	Base            string `json:"base"`
	BaseUnit        string `json:"base_unit"`
	WastePercent    string `json:"waste_percent"`
	WasteAmount     string `json:"waste_amount"`
	WithWaste       string `json:"with_waste"`
	CoveragePerUnit string `json:"coverage_per_unit,omitempty"`
	UnroundedCount  string `json:"unrounded_count,omitempty"`
	UnitsPerPackage int64  `json:"units_per_package,omitempty"`
}

type ConstructionResult struct {
	Operation    ConstructionOperation    `json:"operation"`
	Quantity     string                   `json:"quantity"`
	Unit         string                   `json:"unit"`
	Formula      string                   `json:"formula"`
	PackageCount int64                    `json:"package_count,omitempty"`
	Intermediate ConstructionIntermediate `json:"intermediate"`
}

// CalculateConstruction estimates coverage using caller-supplied dimensions and
// explicit waste in [0, 100] percent. It never rounds intermediate measures.
// Whole-item and package counts round up. Limits are 10,000 m per dimension,
// that bound raised to the matching power for package coverage, and 1e9 pieces
// or packages. Layout, openings, joints and installation are outside the model.
func CalculateConstruction(in ConstructionInput) (ConstructionResult, error) {
	switch in.Operation {
	case ConstructionArea, ConstructionVolume, ConstructionTiles, ConstructionBlocks, ConstructionPackages:
	default:
		return ConstructionResult{}, fmt.Errorf("operation: %w", ErrUnsupported)
	}
	itemCount := in.Operation == ConstructionTiles || in.Operation == ConstructionBlocks
	packages := in.Operation == ConstructionPackages
	hasHeight := in.Height != (ConstructionMeasure{})
	volume := in.Operation == ConstructionVolume || (packages && hasHeight)
	hasItemDetails := in.ItemLength != (ConstructionMeasure{}) || in.ItemWidth != (ConstructionMeasure{}) || in.UnitsPerPackage != 0
	if hasHeight != volume || (!itemCount && hasItemDetails) ||
		(!packages && in.PackageCoverage != (ConstructionMeasure{})) ||
		in.UnitsPerPackage < 0 || in.UnitsPerPackage > maxConstructionCount {
		return ConstructionResult{}, fmt.Errorf("operation inputs: %w", ErrInvalid)
	}

	waste, ok := constructionNumber(in.WastePercent)
	if !ok || waste.Sign() < 0 || waste.Cmp(big.NewRat(maxConstructionWaste, 1)) > 0 {
		return ConstructionResult{}, fmt.Errorf("waste_percent: %w", ErrInvalid)
	}
	length, err := constructionMeasure("length", in.Length, 1)
	if err != nil {
		return ConstructionResult{}, err
	}
	width, err := constructionMeasure("width", in.Width, 1)
	if err != nil {
		return ConstructionResult{}, err
	}

	power, unit := 2, "m2"
	base := new(big.Rat).Mul(length, width)
	trace := ConstructionIntermediate{
		LengthM: constructionDecimal(length), WidthM: constructionDecimal(width),
		WastePercent: constructionDecimal(waste),
	}
	baseFormula := "length_m * width_m"
	if volume {
		height, err := constructionMeasure("height", in.Height, 1)
		if err != nil {
			return ConstructionResult{}, err
		}
		power, unit = 3, "m3"
		base.Mul(base, height)
		trace.HeightM = constructionDecimal(height)
		baseFormula += " * height_m"
	}

	wasteAmount := new(big.Rat).Mul(base, waste)
	wasteAmount.Quo(wasteAmount, big.NewRat(100, 1))
	adjusted := new(big.Rat).Add(base, wasteAmount)
	trace.Base = constructionDecimal(base)
	trace.BaseUnit = unit
	trace.WasteAmount = constructionDecimal(wasteAmount)
	trace.WithWaste = constructionDecimal(adjusted)
	formula := "with_waste = (" + baseFormula + ") * (1 + waste_percent / 100)"
	result := ConstructionResult{Operation: in.Operation, Unit: unit, Quantity: trace.WithWaste, Formula: formula}
	if itemCount || packages {
		var coverage *big.Rat
		if itemCount {
			itemLength, err := constructionMeasure("item_length", in.ItemLength, 1)
			if err != nil {
				return ConstructionResult{}, err
			}
			itemWidth, err := constructionMeasure("item_width", in.ItemWidth, 1)
			if err != nil {
				return ConstructionResult{}, err
			}
			trace.ItemLengthM, trace.ItemWidthM = constructionDecimal(itemLength), constructionDecimal(itemWidth)
			coverage = new(big.Rat).Mul(itemLength, itemWidth)
			result.Unit = "piece"
			result.Formula += "; coverage_per_unit = item_length_m * item_width_m"
		} else {
			coverage, err = constructionMeasure("package_coverage", in.PackageCoverage, power)
			if err != nil {
				return ConstructionResult{}, err
			}
			result.Unit = "pack"
		}
		trace.CoveragePerUnit = constructionDecimal(coverage)
		rawCount := new(big.Rat).Quo(adjusted, coverage)
		trace.UnroundedCount = rawCount.RatString()
		count, err := constructionCeiling(rawCount)
		if err != nil {
			return ConstructionResult{}, err
		}
		result.Quantity = fmt.Sprint(count)
		result.Formula += "; quantity = ceil(with_waste / coverage_per_unit)"
		if packages {
			result.PackageCount = count
		}
		if in.UnitsPerPackage > 0 {
			trace.UnitsPerPackage = in.UnitsPerPackage
			result.PackageCount = (count + in.UnitsPerPackage - 1) / in.UnitsPerPackage
			result.Formula += "; package_count = ceil(quantity / units_per_package)"
		}
	} else {
		result.Formula += "; quantity = with_waste"
	}
	result.Intermediate = trace
	return result, nil
}

func constructionNumber(value string) (*big.Rat, bool) {
	// Match the domain decimal grammar without scanning an unbounded payload.
	if len(value) > 29 {
		return nil, false
	}
	return number(value)
}

func constructionMeasure(field string, measure ConstructionMeasure, power int) (*big.Rat, error) {
	value, ok := constructionNumber(measure.Value)
	if !ok || value.Sign() <= 0 {
		return nil, fmt.Errorf("%s value: %w", field, ErrInvalid)
	}
	suffix := ""
	if power > 1 {
		suffix = fmt.Sprint(power)
	}
	var scale int64
	switch measure.Unit {
	case "m" + suffix:
		scale = 1
	case "cm" + suffix:
		scale = 100
	case "mm" + suffix:
		scale = 1000
	default:
		return nil, fmt.Errorf("%s unit: %w", field, ErrUnsupported)
	}
	divisor, limit := int64(1), int64(1)
	for range power {
		divisor *= scale
		limit *= maxConstructionLengthM
	}
	value.Quo(value, big.NewRat(divisor, 1))
	if value.Cmp(big.NewRat(limit, 1)) > 0 {
		return nil, fmt.Errorf("%s exceeds measure limit: %w", field, ErrInvalid)
	}
	return value, nil
}

func constructionDecimal(value *big.Rat) string {
	// Conversions, products, sums and percentages only use powers of ten;
	// the potentially repeating coverage quotient uses RatString instead.
	places, _ := value.FloatPrec()
	return value.FloatString(places)
}

func constructionCeiling(value *big.Rat) (int64, error) {
	var count, remainder big.Int
	count.QuoRem(value.Num(), value.Denom(), &remainder)
	if remainder.Sign() != 0 {
		count.Add(&count, big.NewInt(1))
	}
	if !count.IsInt64() || count.Int64() > maxConstructionCount {
		return 0, fmt.Errorf("quantity exceeds count limit: %w", ErrInvalid)
	}
	return count.Int64(), nil
}
