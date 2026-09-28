package registry

import (
	_ "embed"
	"encoding/json"
	"strings"

	log "github.com/sirupsen/logrus"
)

// factoryOverlayJSON lists models the factory needs before the upstream catalog
// carries them. An overlay entry is added only when its ID is absent from the
// loaded catalog, so the upstream definition wins as soon as it appears and the
// remote refresh keeps working (the catalog is never frozen by this file).
//
//go:embed models/factory_overlay.json
var factoryOverlayJSON []byte

var factoryOverlay = parseFactoryOverlay(factoryOverlayJSON)

func parseFactoryOverlay(data []byte) *staticModelsJSON {
	var overlay staticModelsJSON
	if err := json.Unmarshal(data, &overlay); err != nil {
		log.Warnf("registry: factory overlay ignored, decode failed: %v", err)
		return nil
	}
	return &overlay
}

func catalogSections(data *staticModelsJSON) []*[]*ModelInfo {
	return []*[]*ModelInfo{
		&data.Claude, &data.Gemini, &data.Vertex, &data.AIStudio,
		&data.CodexFree, &data.CodexTeam, &data.CodexPlus, &data.CodexPro,
		&data.Kimi, &data.Antigravity, &data.XAI, &data.Devin, &data.Meta,
	}
}

// applyFactoryOverlay appends overlay models missing from data, section by section.
func applyFactoryOverlay(data *staticModelsJSON, overlay *staticModelsJSON) {
	if data == nil || overlay == nil {
		return
	}
	targets := catalogSections(data)
	for index, source := range catalogSections(overlay) {
		if len(*source) == 0 {
			continue
		}
		target := targets[index]
		present := make(map[string]struct{}, len(*target))
		for _, model := range *target {
			if model != nil {
				present[strings.ToLower(strings.TrimSpace(model.ID))] = struct{}{}
			}
		}
		for _, model := range *source {
			if model == nil {
				continue
			}
			id := strings.ToLower(strings.TrimSpace(model.ID))
			if id == "" {
				continue
			}
			if _, ok := present[id]; ok {
				continue
			}
			present[id] = struct{}{}
			*target = append(*target, cloneModelInfo(model))
		}
	}
}
