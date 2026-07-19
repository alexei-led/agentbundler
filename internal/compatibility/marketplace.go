package compatibility

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
)

func prepareMarketplace(output model.RelativePath, plan model.TargetPlan) (File, []File, []model.Diagnostic) {
	marker, packageManifest, ok := marketplaceContract(plan.Target)
	if !ok {
		return File{}, nil, []model.Diagnostic{diagnostic("unsupported-compatibility-target", fmt.Sprintf("target %q has no repository-root marketplace contract", plan.Target))}
	}
	catalogFile, exists := targetPlanFile(plan, marker)
	if !exists {
		return File{}, nil, []model.Diagnostic{diagnostic("compatibility-catalog-missing", fmt.Sprintf("target %q did not generate required marketplace %q", plan.Target, marker))}
	}

	var document map[string]json.RawMessage
	if err := model.DecodeStrictJSONObject(catalogFile.Bytes, &document); err != nil {
		return File{}, nil, []model.Diagnostic{diagnostic("compatibility-catalog-invalid", fmt.Sprintf("target %q marketplace: %v", plan.Target, err))}
	}
	encodedPlugins, exists := document["plugins"]
	if !exists {
		return File{}, nil, []model.Diagnostic{diagnostic("compatibility-catalog-invalid", fmt.Sprintf("target %q marketplace has no plugins array", plan.Target))}
	}
	var plugins []json.RawMessage
	if err := model.DecodeStrictJSON(encodedPlugins, &plugins); err != nil || len(plugins) == 0 {
		if err == nil {
			err = fmt.Errorf("must not be empty")
		}
		return File{}, nil, []model.Diagnostic{diagnostic("compatibility-catalog-invalid", fmt.Sprintf("target %q marketplace plugins: %v", plan.Target, err))}
	}

	seenIDs := make(map[string]string, len(plugins))
	seenLocalRoots := make(map[string]string, len(plugins))
	rewritten := make([]json.RawMessage, 0, len(plugins))
	for index, raw := range plugins {
		var entry map[string]json.RawMessage
		if err := model.DecodeStrictJSONObject(raw, &entry); err != nil {
			return File{}, nil, []model.Diagnostic{diagnostic("compatibility-catalog-invalid", fmt.Sprintf("target %q marketplace plugin %d: %v", plan.Target, index, err))}
		}
		var name string
		if encoded, exists := entry["name"]; !exists || model.DecodeStrictJSON(encoded, &name) != nil || strings.TrimSpace(name) == "" {
			return File{}, nil, []model.Diagnostic{diagnostic("compatibility-catalog-invalid", fmt.Sprintf("target %q marketplace plugin %d requires a non-empty string name", plan.Target, index))}
		}
		key := strings.ToLower(name)
		if previous, duplicate := seenIDs[key]; duplicate {
			return File{}, nil, []model.Diagnostic{diagnostic("compatibility-duplicate-id", fmt.Sprintf("target %q marketplace plugin IDs %q and %q collide", plan.Target, previous, name))}
		}
		seenIDs[key] = name

		source, exists := entry["source"]
		if !exists {
			return File{}, nil, []model.Diagnostic{diagnostic("compatibility-catalog-invalid", fmt.Sprintf("target %q marketplace plugin %q has no source", plan.Target, name))}
		}
		rewrittenSource, localRoot, local, err := rewriteMarketplaceSource(plan.Target, output, source)
		if err != nil {
			return File{}, nil, []model.Diagnostic{diagnostic("compatibility-source-unsafe", fmt.Sprintf("target %q marketplace plugin %q: %v", plan.Target, name, err))}
		}
		if local {
			rootKey := strings.ToLower(localRoot)
			if previous, duplicate := seenLocalRoots[rootKey]; duplicate {
				return File{}, nil, []model.Diagnostic{diagnostic("compatibility-source-collision", fmt.Sprintf("target %q marketplace plugins %q and %q resolve to source %q", plan.Target, previous, name, localRoot))}
			}
			seenLocalRoots[rootKey] = name
			expectedManifest := packageManifest
			if localRoot != "." {
				expectedManifest = path.Join(localRoot, packageManifest)
			}
			if !targetPlanHasFile(plan, expectedManifest) {
				return File{}, nil, []model.Diagnostic{diagnostic("compatibility-source-dangling", fmt.Sprintf("target %q marketplace plugin %q source %q has no generated manifest %q", plan.Target, name, localRoot, expectedManifest))}
			}
			entry["source"] = rewrittenSource
		}
		encoded, err := json.Marshal(entry)
		if err != nil {
			return File{}, nil, []model.Diagnostic{diagnostic("compatibility-catalog-invalid", err.Error())}
		}
		rewritten = append(rewritten, encoded)
	}
	encodedPlugins, err := json.Marshal(rewritten)
	if err != nil {
		return File{}, nil, []model.Diagnostic{diagnostic("compatibility-catalog-invalid", err.Error())}
	}
	document["plugins"] = encodedPlugins
	data, err := json.Marshal(document)
	if err != nil {
		return File{}, nil, []model.Diagnostic{diagnostic("compatibility-catalog-invalid", err.Error())}
	}

	extra := make([]File, 0)
	if plan.Target == model.TargetCodex {
		for _, planned := range plan.Files {
			if !strings.HasPrefix(string(planned.Path), ".codex/agents/") {
				continue
			}
			extra = append(extra, File{Path: planned.Path, Bytes: append([]byte(nil), planned.Bytes...)})
		}
		sort.Slice(extra, func(left, right int) bool { return extra[left].Path < extra[right].Path })
	}
	return File{Path: marker, Bytes: append(data, '\n')}, extra, nil
}

func marketplaceContract(target model.TargetID) (model.RelativePath, string, bool) {
	switch target {
	case model.TargetClaude, model.TargetGrok:
		return ".claude-plugin/marketplace.json", ".claude-plugin/plugin.json", true
	case model.TargetCodex:
		return ".agents/plugins/marketplace.json", ".codex-plugin/plugin.json", true
	case model.TargetCopilot:
		return ".github/plugin/marketplace.json", "plugin.json", true
	case model.TargetCursor:
		return ".cursor-plugin/marketplace.json", ".cursor-plugin/plugin.json", true
	default:
		return "", "", false
	}
}

func rewriteMarketplaceSource(target model.TargetID, output model.RelativePath, raw json.RawMessage) (json.RawMessage, string, bool, error) {
	if target == model.TargetCodex {
		var object map[string]json.RawMessage
		if err := model.DecodeStrictJSONObject(raw, &object); err != nil {
			return raw, "", false, nil
		}
		var kind string
		if encoded, exists := object["source"]; !exists || model.DecodeStrictJSON(encoded, &kind) != nil || kind != "local" {
			return raw, "", false, nil
		}
		var value string
		encodedPath, exists := object["path"]
		if !exists || model.DecodeStrictJSON(encodedPath, &value) != nil {
			return nil, "", false, fmt.Errorf("local source object requires a string path")
		}
		local, err := safeLocalSource(value)
		if err != nil {
			return nil, "", false, err
		}
		object["path"], _ = json.Marshal(rootRoute(output, target, local))
		encoded, err := json.Marshal(object)
		return encoded, local, true, err
	}

	var value string
	if err := model.DecodeStrictJSON(raw, &value); err != nil {
		var object map[string]json.RawMessage
		if objectErr := model.DecodeStrictJSONObject(raw, &object); objectErr == nil && len(object) != 0 {
			return raw, "", false, nil
		}
		return nil, "", false, fmt.Errorf("source must be a string or non-empty remote object")
	}
	if remoteSource(value) {
		return raw, "", false, nil
	}
	local, err := safeLocalSource(value)
	if err != nil {
		return nil, "", false, err
	}
	encoded, err := json.Marshal(rootRoute(output, target, local))
	return encoded, local, true, err
}

func remoteSource(value string) bool {
	lower := strings.ToLower(value)
	if strings.HasPrefix(value, "git@") || strings.HasPrefix(lower, "github:") || strings.HasPrefix(lower, "npm:") {
		return true
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return false
	}
	switch parsed.Scheme {
	case "https", "http", "git", "ssh":
		return true
	default:
		return false
	}
}
