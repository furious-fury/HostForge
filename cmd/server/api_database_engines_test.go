package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDatabaseEngineCatalogContract(t *testing.T) {
	recorder := httptest.NewRecorder()
	(&server{}).handleDatabaseEngines(recorder, httptest.NewRequest(http.MethodGet, "/api/database-engines", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Engines []struct {
			ID                    string `json:"id"`
			PublicAccessAvailable bool   `json:"public_access_available"`
		} `json:"engines"`
		ResourcePresets []struct {
			ID string `json:"id"`
		} `json:"resource_presets"`
		Networking struct {
			Scope                 string `json:"scope"`
			PublicAccessAvailable bool   `json:"public_access_available"`
		} `json:"networking"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Engines) != 6 || len(payload.ResourcePresets) != 4 {
		t.Fatalf("engines=%d presets=%d", len(payload.Engines), len(payload.ResourcePresets))
	}
	if payload.Networking.Scope != "hostforge_environment" || payload.Networking.PublicAccessAvailable {
		t.Fatalf("unexpected networking contract: %+v", payload.Networking)
	}
	for _, engine := range payload.Engines {
		if engine.PublicAccessAvailable {
			t.Fatalf("public access enabled for %s", engine.ID)
		}
	}
}
