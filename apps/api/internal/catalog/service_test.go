package catalog

import "testing"

func TestValidation(t *testing.T) {
	for _, in := range []Input{{SKU: "bad space", PriceAmount: 1}, {SKU: "OK", PriceAmount: -1}, {SKU: "OK", PriceAmount: 9000000000001}, {SKU: "OK", CurrencyCode: "USD"}, {SKU: "OK", Attributes: map[string]string{"": "bad"}}} {
		if in.validateVariant() == nil {
			t.Fatal("invalid variant accepted", in)
		}
	}
	in := Input{SKU: "  test-1  ", PriceAmount: 12345}
	if e := in.validateVariant(); e != nil || in.SKU != "TEST-1" || in.CurrencyCode != "RUB" {
		t.Fatal("normalization", e)
	}
	for _, f := range []Filters{{PageSize: 101}, {Page: -1}, {Sort: "SQL"}, {Status: "deleted"}, {Stock: "maybe"}} {
		if validateFilters(&f) == nil {
			t.Fatal("invalid filter accepted")
		}
	}
}
