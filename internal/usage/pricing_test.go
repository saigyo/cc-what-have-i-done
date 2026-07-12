package usage

import "testing"

func TestLookupKnownModel(t *testing.T) {
	p, ok := Lookup("claude-opus-4-8")
	if !ok {
		t.Fatal("claude-opus-4-8 not found")
	}
	if p.Input != 5 || p.Output != 25 {
		t.Errorf("price = %+v, want {5 25}", p)
	}
}

func TestLookupStripsDateSuffix(t *testing.T) {
	if _, ok := Lookup("claude-haiku-4-5-20251001"); !ok {
		t.Error("dated model id should resolve after stripping the date suffix")
	}
}

func TestLookupUnknownAndSynthetic(t *testing.T) {
	if _, ok := Lookup("<synthetic>"); ok {
		t.Error("<synthetic> should not be priced")
	}
	if _, ok := Lookup("gpt-4"); ok {
		t.Error("unknown model should not be priced")
	}
}

func TestPricesAsOfSet(t *testing.T) {
	if PricesAsOf == "" {
		t.Error("PricesAsOf must be set")
	}
}
