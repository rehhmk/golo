package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/enzotriches/golo/internal/eventstore"
	"github.com/enzotriches/golo/internal/publisher"
	"github.com/enzotriches/golo/internal/scenario"
	"github.com/enzotriches/golo/internal/signals"
)

func TestLockedTestAPISealsContractAndRedactsOutcomes(t *testing.T) {
	store, err := eventstore.NewSQLiteStore(filepath.Join(t.TempDir(), "locked-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.SaveSignalSettings(signals.DefaultSettings()); err != nil {
		t.Fatal(err)
	}
	definition := scenario.DefaultDefinition()
	definition.ID, definition.Name, definition.Enabled = "api-strategy", "API strategy", true
	definition.Conditions = []scenario.Condition{{
		Field: scenario.FieldMinute, Operator: scenario.OpGreaterOrEqual, Value: 70,
	}}
	report := scenario.QualificationReport{
		ValidationQualified: true, ModelValidationQualified: true,
		Partition: scenario.PartitionManifest{SHA256: "partition-sha"},
	}
	if err := store.SaveStrategy(signals.StoredStrategy{Definition: definition, Report: report}); err != nil {
		t.Fatal(err)
	}
	model := signals.ModelContract{
		ModelVersion: "model-v1", ModelSHA256: "model-sha",
		FeatureVersion: "features-v1", OneGoalQualified: true,
	}
	server := NewServerWithAdmin(store, publisher.NewPublisher("", ""), nil, model.ModelVersion, AdminDependencies{
		ModelContract: model,
	})

	sealed := httptest.NewRecorder()
	server.handleLockedTestRoute(sealed, httptest.NewRequest(http.MethodPost,
		"/api/admin/strategies/api-strategy/versions/1/seal", nil),
		"strategies/api-strategy/versions/1/seal")
	if sealed.Code != http.StatusOK {
		t.Fatalf("seal returned %d: %s", sealed.Code, sealed.Body.String())
	}
	if strings.Contains(sealed.Body.String(), `"report"`) ||
		strings.Contains(sealed.Body.String(), `"wins"`) ||
		strings.Contains(sealed.Body.String(), `"hitRate"`) {
		t.Fatalf("collecting response leaked outcome data: %s", sealed.Body.String())
	}
	if !strings.Contains(sealed.Body.String(), `"state":"COLLECTING"`) ||
		!strings.Contains(sealed.Body.String(), `"partitionManifestSha256":"partition-sha"`) {
		t.Fatalf("sealed contract missing state or manifest: %s", sealed.Body.String())
	}

	collecting := httptest.NewRecorder()
	server.handleLockedTestRoute(collecting, httptest.NewRequest(http.MethodGet,
		"/api/admin/strategies/api-strategy/versions/1/locked-test", nil),
		"strategies/api-strategy/versions/1/locked-test")
	if collecting.Code != http.StatusOK || strings.Contains(collecting.Body.String(), `"report"`) {
		t.Fatalf("GET leaked report or failed: %d %s", collecting.Code, collecting.Body.String())
	}

	reveal := httptest.NewRecorder()
	server.handleLockedTestRoute(reveal, httptest.NewRequest(http.MethodPost,
		"/api/admin/strategies/api-strategy/versions/1/reveal", nil),
		"strategies/api-strategy/versions/1/reveal")
	if reveal.Code != http.StatusBadRequest {
		t.Fatalf("premature reveal returned %d: %s", reveal.Code, reveal.Body.String())
	}
}
