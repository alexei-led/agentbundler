package agentplugin

import (
	"sort"
	"strings"

	"github.com/alexei-led/agentbundler/internal/compiler/model"
	"github.com/alexei-led/agentbundler/internal/compiler/source/frontmatter"
)

// discoverSkills finds Agent Skills at the immediate children of the plugin
// root. A skill is a directory at depth 1 that contains a SKILL.md file.
// The skill name is the directory name; the identity is skill/<name>.
// It returns discovered skill assets and the set of plugin-relative file paths
// consumed by skills (SKILL.md plus support files), so callers can exclude them
// from AgentPluginData.PackageFiles.
func discoverSkills(files []traversedFile, workspacePrefix string) ([]model.SourceAsset, map[string]bool) {
	// Collect per-skill: SKILL.md bytes and support files.
	type skillEntry struct {
		skillMD traversedFile
		support []traversedFile
	}
	skills := make(map[string]*skillEntry)
	for _, f := range files {
		parts := strings.SplitN(f.relPath, "/", 3)
		if len(parts) < 2 {
			// root-level file; not a skill file
			continue
		}
		name := parts[0]
		rest := parts[1]
		if len(parts) == 2 && rest == "SKILL.md" {
			if _, ok := skills[name]; !ok {
				skills[name] = &skillEntry{}
			}
			skills[name].skillMD = f
		}
	}

	// Collect support files for directories that have a SKILL.md.
	for _, f := range files {
		parts := strings.SplitN(f.relPath, "/", 3)
		if len(parts) < 2 {
			continue
		}
		name := parts[0]
		rest := strings.Join(parts[1:], "/")
		if entry, ok := skills[name]; ok && rest != "SKILL.md" {
			entry.support = append(entry.support, f)
		}
	}

	// Build the used-paths set and skill assets.
	used := make(map[string]bool)
	var assets []model.SourceAsset

	sortedNames := make([]string, 0, len(skills))
	for name := range skills {
		if skills[name].skillMD.relPath == "" {
			// No SKILL.md found for this entry (shouldn't happen but guard).
			continue
		}
		sortedNames = append(sortedNames, name)
	}
	sort.Strings(sortedNames)

	for _, name := range sortedNames {
		entry := skills[name]

		identity, err := model.NewAssetID("skill/" + name)
		if err != nil {
			// Invalid skill name; skip silently (will appear in PackageFiles).
			continue
		}

		// Parse SKILL.md frontmatter and body.
		fm, body, err := frontmatter.Parse(entry.skillMD.bytes)
		if err != nil {
			// Unparseable SKILL.md; skip silently.
			continue
		}

		// Mark SKILL.md as used.
		used[entry.skillMD.relPath] = true

		// Build support files map (relative to skill root).
		assetFiles := make(map[model.RelativePath]model.FileContent, len(entry.support))
		for _, sf := range entry.support {
			// relPath relative to the skill root: strip "<name>/"
			relToSkill := strings.TrimPrefix(sf.relPath, name+"/")
			rp, err := model.NewRelativePath(relToSkill)
			if err != nil {
				// Invalid path; skip.
				continue
			}
			origin := workspaceOrigin(workspacePrefix, sf.relPath)
			assetFiles[rp] = model.FileContent{
				Bytes:      sf.bytes,
				Executable: sf.executable,
				Origin:     []model.SourceLocation{{Path: origin}},
			}
			used[sf.relPath] = true
		}

		skillLoc := model.SourceLocation{Path: workspaceOrigin(workspacePrefix, entry.skillMD.relPath)}
		assets = append(assets, model.SourceAsset{
			Identity: identity,
			Kind:     model.AssetKindSkill,
			Base: model.AssetContent{
				Frontmatter: fm,
				Body:        body,
				Files:       assetFiles,
			},
			CapabilityUses: []model.CapabilityUse{{
				Key:      model.CapabilityKeyAgentPluginSkills,
				Location: skillLoc,
			}},
		})
	}

	return assets, used
}

// workspaceOrigin builds a workspace-relative path from the plugin's workspace
// prefix and the file's plugin-relative path.
func workspaceOrigin(workspacePrefix, pluginRelPath string) model.RelativePath {
	if workspacePrefix == "" {
		return model.RelativePath(pluginRelPath)
	}
	return model.RelativePath(workspacePrefix + "/" + pluginRelPath)
}
