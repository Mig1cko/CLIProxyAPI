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
	if got := countModelID(data.Claude, "shared-id"); got != 1 {
		t.Fatalf("claude section count = %d, want 1 (overlay must not leak into other sections)", got)
	}
	for _, section := range catalogSections(data)[2:] {
		if len(*section) != 0 {
			t.Fatalf("overlay leaked into an unrelated section: %+v", *section)
		}
	}
}

// factoryOverlayClaudeIDs are the models the factory build must serve even when the
// remote catalog is unreachable: Sonnet 5.5 (claude-only layout) and Haiku 5.5, the
// specialists' model since 2026-10-08.
var factoryOverlayClaudeIDs = []string{"claude-sonnet-5-5", "claude-haiku-5-5"}

func TestEmbeddedCatalogCarriesOverlayAfterLoad(t *testing.T) {
	for _, id := range factoryOverlayClaudeIDs {
		if factoryOverlay == nil || countModelID(factoryOverlay.Claude, id) != 1 {
			t.Fatalf("embedded factory overlay must declare %s", id)
		}
	}
	restoreCatalogAfter(t)
	if err := loadModelsFromBytes(embeddedModelsJSON, "embed"); err != nil {
		t.Fatalf("load embedded catalog: %v", err)
	}
	for _, id := range factoryOverlayClaudeIDs {
		if got := countModelID(GetClaudeModels(), id); got != 1 {
			t.Fatalf("claude catalog count for %s = %d, want 1", id, got)
		}
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

	for _, id := range factoryOverlayClaudeIDs {
		if got := countModelID(GetClaudeModels(), id); got != 1 {
			t.Fatalf("after remote refresh %s count = %d, want 1", id, got)
		}
	}
}

func TestRefreshWithUnchangedUpstreamReportsNoClaudeChange(t *testing.T) {
	restoreCatalogAfter(t)
	if err := loadModelsFromBytes(embeddedModelsJSON, "embed"); err != nil {
		t.Fatalf("load embedded catalog: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(embeddedModelsJSON)
	}))
	defer server.Close()
	previous := modelsURLs
	modelsURLs = []string{server.URL}
	defer func() { modelsURLs = previous }()

	refreshCallbackMu.Lock()
	previousCallback := refreshCallback
	refreshCallbackMu.Unlock()
	var changed []string
	SetModelRefreshCallback(func(providers []string) { changed = append(changed, providers...) })
	defer SetModelRefreshCallback(previousCallback)

	// The overlay must be applied before change detection; otherwise every
	// refresh would report claude as changed and re-register its models.
	tryRefreshModels(context.Background(), "test refresh")

	for _, provider := range changed {
		if provider == "claude" {
			t.Fatalf("unchanged upstream catalog reported claude as changed: %v", changed)
		}
	}
}
