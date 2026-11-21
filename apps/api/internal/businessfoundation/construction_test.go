package businessfoundation

import (
	"errors"
	"strings"
	"testing"
)

func TestCalculateConstruction(t *testing.T) {
	tests := []struct {
		name string
		in   ConstructionInput
		want ConstructionResult
	}{
		{
			name: "mixed length units and fractional waste",
			in: ConstructionInput{
				Operation: ConstructionArea, Length: ConstructionMeasure{"3000", "mm"},
				Width: ConstructionMeasure{"250.5", "cm"}, WastePercent: "12.5",
			},
			want: ConstructionResult{
				Operation: ConstructionArea, Quantity: "8.454375", Unit: "m2",
				Formula: "with_waste = (length_m * width_m) * (1 + waste_percent / 100); quantity = with_waste",
				Intermediate: ConstructionIntermediate{
					LengthM: "3", WidthM: "2.505", Base: "7.515", BaseUnit: "m2",
					WastePercent: "12.5", WasteAmount: "0.939375", WithWaste: "8.454375",
				},
			},
		},
		{
			name: "volume keeps maximum input precision",
			in: ConstructionInput{
				Operation: ConstructionVolume, Length: ConstructionMeasure{"0.000000001", "mm"},
				Width: ConstructionMeasure{"0.000000001", "mm"}, Height: ConstructionMeasure{"0.000000001", "mm"},
				WastePercent: "0.000000001",
			},
			want: ConstructionResult{
				Operation: ConstructionVolume, Quantity: "0." + strings.Repeat("0", 35) + "100000000001", Unit: "m3",
				Formula: "with_waste = (length_m * width_m * height_m) * (1 + waste_percent / 100); quantity = with_waste",
				Intermediate: ConstructionIntermediate{
					LengthM: "0.000000000001", WidthM: "0.000000000001", HeightM: "0.000000000001",
					Base: "0." + strings.Repeat("0", 35) + "1", BaseUnit: "m3", WastePercent: "0.000000001",
					WasteAmount: "0." + strings.Repeat("0", 46) + "1",
					WithWaste:   "0." + strings.Repeat("0", 35) + "100000000001",
				},
			},
		},
		{
			name: "tiles round after waste then round whole packs",
			in: ConstructionInput{
				Operation: ConstructionTiles, Length: ConstructionMeasure{"2", "m"}, Width: ConstructionMeasure{"1", "m"},
				ItemLength: ConstructionMeasure{"300", "mm"}, ItemWidth: ConstructionMeasure{"50", "cm"},
				UnitsPerPackage: 6, WastePercent: "10",
			},
			want: ConstructionResult{
				Operation: ConstructionTiles, Quantity: "15", Unit: "piece", PackageCount: 3,
				Formula: "with_waste = (length_m * width_m) * (1 + waste_percent / 100); coverage_per_unit = item_length_m * item_width_m; quantity = ceil(with_waste / coverage_per_unit); package_count = ceil(quantity / units_per_package)",
				Intermediate: ConstructionIntermediate{
					LengthM: "2", WidthM: "1", ItemLengthM: "0.3", ItemWidthM: "0.5",
					Base: "2", BaseUnit: "m2", WastePercent: "10", WasteAmount: "0.2", WithWaste: "2.2",
					CoveragePerUnit: "0.15", UnroundedCount: "44/3", UnitsPerPackage: 6,
				},
			},
		},
		{
			name: "blocks exact whole count",
			in: ConstructionInput{
				Operation: ConstructionBlocks, Length: ConstructionMeasure{"2.4", "m"}, Width: ConstructionMeasure{"1000", "mm"},
				ItemLength: ConstructionMeasure{"60", "cm"}, ItemWidth: ConstructionMeasure{"200", "mm"}, WastePercent: "0",
			},
			want: ConstructionResult{
				Operation: ConstructionBlocks, Quantity: "20", Unit: "piece",
				Formula: "with_waste = (length_m * width_m) * (1 + waste_percent / 100); coverage_per_unit = item_length_m * item_width_m; quantity = ceil(with_waste / coverage_per_unit)",
				Intermediate: ConstructionIntermediate{
					LengthM: "2.4", WidthM: "1", ItemLengthM: "0.6", ItemWidthM: "0.2",
					Base: "2.4", BaseUnit: "m2", WastePercent: "0", WasteAmount: "0", WithWaste: "2.4",
					CoveragePerUnit: "0.12", UnroundedCount: "20",
				},
			},
		},
		{
			name: "square package coverage converts before division",
			in: ConstructionInput{
				Operation: ConstructionPackages, Length: ConstructionMeasure{"1", "m"}, Width: ConstructionMeasure{"100", "cm"},
				PackageCoverage: ConstructionMeasure{"3000", "cm2"}, WastePercent: "0",
			},
			want: ConstructionResult{
				Operation: ConstructionPackages, Quantity: "4", Unit: "pack", PackageCount: 4,
				Formula: "with_waste = (length_m * width_m) * (1 + waste_percent / 100); quantity = ceil(with_waste / coverage_per_unit)",
				Intermediate: ConstructionIntermediate{
					LengthM: "1", WidthM: "1", Base: "1", BaseUnit: "m2", WastePercent: "0", WasteAmount: "0",
					WithWaste: "1", CoveragePerUnit: "0.3", UnroundedCount: "10/3",
				},
			},
		},
		{
			name: "cubic package coverage and just above whole count",
			in: ConstructionInput{
				Operation: ConstructionPackages, Length: ConstructionMeasure{"1", "m"}, Width: ConstructionMeasure{"1", "m"},
				Height: ConstructionMeasure{"1", "mm"}, PackageCoverage: ConstructionMeasure{"1000000", "mm3"},
				WastePercent: "0.000000001",
			},
			want: ConstructionResult{
				Operation: ConstructionPackages, Quantity: "2", Unit: "pack", PackageCount: 2,
				Formula: "with_waste = (length_m * width_m * height_m) * (1 + waste_percent / 100); quantity = ceil(with_waste / coverage_per_unit)",
				Intermediate: ConstructionIntermediate{
					LengthM: "1", WidthM: "1", HeightM: "0.001", Base: "0.001", BaseUnit: "m3",
					WastePercent: "0.000000001", WasteAmount: "0.00000000000001", WithWaste: "0.00100000000001",
					CoveragePerUnit: "0.001", UnroundedCount: "100000000001/100000000000",
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for range 2 {
				got, err := CalculateConstruction(test.in)
				if err != nil || got != test.want {
					t.Fatalf("got %+v, %v\nwant %+v", got, err, test.want)
				}
			}
		})
	}
}

func TestCalculateConstructionValidation(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*ConstructionInput)
		err    error
	}{
		{"unknown operation", func(in *ConstructionInput) { in.Operation = "material" }, ErrUnsupported},
		{"waste omitted", func(in *ConstructionInput) { in.WastePercent = "" }, ErrInvalid},
		{"negative waste", func(in *ConstructionInput) { in.WastePercent = "-0.01" }, ErrInvalid},
		{"waste above limit", func(in *ConstructionInput) { in.WastePercent = "100.000000001" }, ErrInvalid},
		{"zero dimension", func(in *ConstructionInput) { in.Length.Value = "0" }, ErrInvalid},
		{"negative dimension", func(in *ConstructionInput) { in.Width.Value = "-1" }, ErrInvalid},
		{"excess precision", func(in *ConstructionInput) { in.Width.Value = "1.0000000001" }, ErrInvalid},
		{"scientific notation", func(in *ConstructionInput) { in.Width.Value = "1e3" }, ErrInvalid},
		{"rational input", func(in *ConstructionInput) { in.Width.Value = "1/3" }, ErrInvalid},
		{"oversized input", func(in *ConstructionInput) { in.Width.Value = strings.Repeat("1", 256) }, ErrInvalid},
		{"missing unit", func(in *ConstructionInput) { in.Width.Unit = "" }, ErrUnsupported},
		{"nonmetric unit", func(in *ConstructionInput) { in.Width.Unit = "ft" }, ErrUnsupported},
		{"area used for length", func(in *ConstructionInput) { in.Width.Unit = "m2" }, ErrUnsupported},
		{"length over limit after conversion", func(in *ConstructionInput) {
			in.Length = ConstructionMeasure{"10000000.1", "mm"}
		}, ErrInvalid},
		{"missing volume height", func(in *ConstructionInput) { in.Operation = ConstructionVolume }, ErrInvalid},
		{"unexpected area height", func(in *ConstructionInput) { in.Height = ConstructionMeasure{"1", "m"} }, ErrInvalid},
		{"unexpected dimensions for area", func(in *ConstructionInput) { in.ItemLength.Unit = "cm" }, ErrInvalid},
		{"missing block dimensions", func(in *ConstructionInput) { in.Operation = ConstructionBlocks }, ErrInvalid},
		{"unexpected pack coverage", func(in *ConstructionInput) { in.PackageCoverage = ConstructionMeasure{"1", "m2"} }, ErrInvalid},
		{"missing package coverage", func(in *ConstructionInput) { in.Operation = ConstructionPackages }, ErrInvalid},
		{"square and cubic mismatch", func(in *ConstructionInput) {
			in.Operation = ConstructionPackages
			in.PackageCoverage = ConstructionMeasure{"1", "m3"}
		}, ErrUnsupported},
		{"zero package coverage", func(in *ConstructionInput) {
			in.Operation = ConstructionPackages
			in.PackageCoverage = ConstructionMeasure{"0", "m2"}
		}, ErrInvalid},
		{"package coverage over limit", func(in *ConstructionInput) {
			in.Operation = ConstructionPackages
			in.PackageCoverage = ConstructionMeasure{"100000000.000000001", "m2"}
		}, ErrInvalid},
		{"pack size over limit", func(in *ConstructionInput) {
			in.Operation = ConstructionTiles
			in.UnitsPerPackage = maxConstructionCount + 1
		}, ErrInvalid},
		{"negative pack size", func(in *ConstructionInput) {
			in.Operation = ConstructionTiles
			in.UnitsPerPackage = -1
		}, ErrInvalid},
		{"count beyond integer range", func(in *ConstructionInput) {
			in.Operation = ConstructionTiles
			in.ItemLength = ConstructionMeasure{"0.000000001", "mm"}
			in.ItemWidth = in.ItemLength
		}, ErrInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			in := ConstructionInput{
				Operation: ConstructionArea, Length: ConstructionMeasure{"1", "m"},
				Width: ConstructionMeasure{"1", "m"}, WastePercent: "0",
			}
			test.change(&in)
			got, err := CalculateConstruction(in)
			if !errors.Is(err, test.err) || got != (ConstructionResult{}) {
				t.Fatalf("got %+v, %v; want zero result and %v", got, err, test.err)
			}
		})
	}
}

func TestCalculateConstructionLimits(t *testing.T) {
	in := ConstructionInput{
		Operation: ConstructionPackages, Length: ConstructionMeasure{"10000000", "mm"},
		Width: ConstructionMeasure{"1000000", "cm"}, PackageCoverage: ConstructionMeasure{"0.2", "m2"},
		WastePercent: "100",
	}
	got, err := CalculateConstruction(in)
	if err != nil || got.PackageCount != maxConstructionCount || got.Quantity != "1000000000" {
		t.Fatalf("exact boundary: got %+v, %v", got, err)
	}

	in.PackageCoverage.Value = "0.199999999"
	got, err = CalculateConstruction(in)
	if !errors.Is(err, ErrInvalid) || got != (ConstructionResult{}) {
		t.Fatalf("over count boundary: got %+v, %v", got, err)
	}
}
