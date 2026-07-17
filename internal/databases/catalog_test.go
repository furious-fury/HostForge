package databases

import "testing"

func TestCatalogContainsSupportedInitialEngines(t *testing.T) {
	expected := map[string]bool{
		"postgresql": false, "mysql": false, "mariadb": false,
		"mongodb": false, "redis": false, "valkey": false,
	}
	for _, engine := range Catalog() {
		if _, ok := expected[engine.ID]; !ok {
			t.Fatalf("unexpected engine %q", engine.ID)
		}
		expected[engine.ID] = true
		if engine.InternalPort <= 0 || engine.ConnectionVariable == "" ||
			engine.MinimumMemoryBytes <= 0 || len(engine.Versions) == 0 {
			t.Fatalf("incomplete engine metadata: %+v", engine)
		}
		defaults := 0
		for _, version := range engine.Versions {
			if version.Default {
				defaults++
			}
			if !version.ProvisioningAvailable || version.ImageRef == "" {
				t.Fatalf("engine %s version %s is not digest-pinned and provisionable", engine.ID, version.Version)
			}
		}
		if defaults != 1 {
			t.Fatalf("engine %s has %d defaults", engine.ID, defaults)
		}
		if engine.PublicAccessAvailable {
			t.Fatalf("public database access must remain disabled for %s", engine.ID)
		}
	}
	for engine, present := range expected {
		if !present {
			t.Fatalf("missing engine %s", engine)
		}
	}
}

func TestCatalogReturnsDefensiveCopies(t *testing.T) {
	first := Catalog()
	first[0].Versions[0].Version = "changed"
	second := Catalog()
	if second[0].Versions[0].Version == "changed" {
		t.Fatal("catalog version slices share mutable state")
	}
}
