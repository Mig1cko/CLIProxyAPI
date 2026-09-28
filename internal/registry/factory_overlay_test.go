package registry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func countModelID(models []*ModelInfo, id string) int {
	count := 0
	for _, model := range models {
		if model != nil && model.ID == id {
			count++
		}
	}
	return count
}

func TestFactoryOverlayAddsMissingModel(t *testing.T) {
	data := &staticModelsJSON{Claude: []*ModelInfo{{ID: "claude-sonnet-5"}}}
	overlay := &staticModelsJSON{Claude: []*ModelInfo{{ID: "claude-sonnet-5-5", DisplayName: "overlay"}}}

	applyFactoryOverlay(data, overlay)

	if got := countModelID(data.Claude, "claude-sonnet-5-5"); got != 1 {
		t.Fatalf("overlay model count = %d, want 1", got)
	}
	if got := countModelID(data.Claude, "claude-sonnet-5"); got != 1 {
		t.Fatalf("existing model count = %d, want 1", got)
	}
}

func TestFactoryOverlayYieldsToUpstreamDefinition(t *testing.T) {
	data := &staticModelsJSON{Claude: []*ModelInfo{{ID: "Claude-Sonnet-5-5", DisplayName: "upstream"}}}
	overlay := &staticModelsJSON{Claude: []*ModelInfo{{ID: "claude-sonnet-5-5", DisplayName: "overlay"}}}

	applyFactoryOverlay(data, overlay)

	if len(data.Claude) != 1 || data.Claude[0].DisplayName != "upstream" {
		t.Fatalf("upstream definition must win, got %+v", data.Claude)
	}
}

func TestFactoryOverlayKeepsSectionsApart(t *testing.T) {
	data := &staticModelsJSON{Claude: []*ModelInfo{{ID: "shared-id"}}}
	overlay := &staticModelsJSON{Gemini: []*ModelInfo{{ID: "shared-id"}}}

	applyFactoryOverlay(data, overlay)

	if got := countModelID(data.Gemini, "shared-id"); got != 1 {
		t.Fatalf("gemini section count = %d, want 1", got)
	}
}

func TestEmbeddedCatalogCarriesOverlayAfterLoad(t *testing.T) {
	if factoryOverlay == nil || countModelID(factoryOverlay.Claude, "claude-sonnet-5-5") != 1 {
		t.Fatalf("embedded factory overlay must declare claude-sonnet-5-5")
	}
	restoreCatalogAfter(t)
	if err := loadModelsFromBytes(embeddedModelsJSON, "embed"); err != nil {
		t.Fatalf("load embedded catalog: %v", err)
	}
	if got := countModelID(GetClaudeModels(), "claude-sonnet-5-5"); got != 1 {
		t.Fatalf("claude catalog count for claude-sonnet-5-5 = %d, want 1", got)
	}
}

func restoreCatalogAfter(t *testing.T) {
	t.Helper()
	saved := getModels()
	t.Cleanup(func() {
		modelsCatalogStore.mu.Lock()
		modelsCatalogStore.data = saved
		modelsCatalogStore.mu.Unlock()
	})
}

func TestRemoteRefreshKeepsOverlayModel(t *testing.T) {
	restoreCatalogAfter(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(embeddedModelsJSON)
	}))
	defer server.Close()

	previous := modelsURLs
	modelsURLs = []string{server.URL}
	defer func() { modelsURLs = previous }()

	tryRefreshModels(context.Background(), "test refresh")

	if got := countModelID(GetClaudeModels(), "claude-sonnet-5-5"); got != 1 {
		t.Fatalf("after remote refresh claude-sonnet-5-5 count = %d, want 1", got)
	}
}
