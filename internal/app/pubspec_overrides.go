package app

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/EndersonPro/flutree/internal/domain"
	"gopkg.in/yaml.v3"
)

const overrideFileName = "pubspec_overrides.yaml"

const emptyOverrideContent = "dependency_overrides:\n  {}\n"

// overrideSet maps a package name to its dependency_overrides spec exactly as
// pub accepts it: a version string, or a map with path, git, hosted, ...
type overrideSet map[string]*yaml.Node

// A pubspec_overrides.yaml replaces the dependency_overrides of pubspec.yaml
// instead of merging with them, so every override the root pubspec declares
// must be carried into the generated file or pub silently drops it.
//
// Precedence, lowest to highest:
//  1. dependency_overrides of the root pubspec.yaml (relative paths rebased
//     from the source repository to the worktree),
//  2. entries already present in the worktree's pubspec_overrides.yaml,
//  3. the package worktrees flutree manages.
func writeOverrideFile(overridePath, worktreePath, sourceRepoRoot string, packages []domain.PlannedWorktree) error {
	pubspecOverrides, err := readOverridesSection(filepath.Join(worktreePath, "pubspec.yaml"))
	if err != nil {
		return err
	}
	rebasePathOverrides(pubspecOverrides, sourceRepoRootOr(sourceRepoRoot, worktreePath), worktreePath)

	existing, err := readOverridesSection(overridePath)
	if err != nil {
		return err
	}

	merged := mergeOverrides(pubspecOverrides, existing, packageOverrides(worktreePath, packages))
	content, err := formatOverrides(merged)
	if err != nil {
		return err
	}
	if current, err := os.ReadFile(overridePath); err == nil && string(current) == content {
		return nil
	}
	return os.WriteFile(overridePath, []byte(content), 0o644)
}

// buildOverrideContent renders the override file shown in the dry plan, before
// the root worktree exists, from the source repository's pubspec.yaml.
func buildOverrideContent(root domain.PlannedWorktree, packages []domain.PlannedWorktree) string {
	pubspecOverrides, err := readOverridesSection(filepath.Join(root.Repo.RepoRoot, "pubspec.yaml"))
	if err != nil {
		pubspecOverrides = overrideSet{}
	}
	rebasePathOverrides(pubspecOverrides, root.Repo.RepoRoot, root.Path)
	content, err := formatOverrides(mergeOverrides(pubspecOverrides, packageOverrides(root.Path, packages)))
	if err != nil {
		return emptyOverrideContent
	}
	return content
}

// readOverridesSection returns the dependency_overrides of a pubspec-style
// file. A missing file or section yields an empty set.
func readOverridesSection(path string) (overrideSet, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return overrideSet{}, nil
	}
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	set := overrideSet{}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return set, nil
	}
	top := doc.Content[0]
	for i := 0; i+1 < len(top.Content); i += 2 {
		if top.Content[i].Value != "dependency_overrides" {
			continue
		}
		section := top.Content[i+1]
		if section.Kind != yaml.MappingNode {
			return set, nil
		}
		for j := 0; j+1 < len(section.Content); j += 2 {
			set[section.Content[j].Value] = section.Content[j+1]
		}
	}
	return set, nil
}

// rebasePathOverrides rewrites relative `path:` overrides written relative to
// fromDir so they point at the same directory when read from toDir. Paths that
// stay inside fromDir are left untouched: they point into the checkout itself.
func rebasePathOverrides(set overrideSet, fromDir, toDir string) {
	if filepath.Clean(fromDir) == filepath.Clean(toDir) {
		return
	}
	for _, spec := range set {
		if spec.Kind != yaml.MappingNode {
			continue
		}
		for i := 0; i+1 < len(spec.Content); i += 2 {
			key, value := spec.Content[i], spec.Content[i+1]
			if key.Value != "path" || value.Kind != yaml.ScalarNode || value.Value == "" || filepath.IsAbs(value.Value) {
				continue
			}
			target := filepath.Join(fromDir, filepath.FromSlash(value.Value))
			// A path inside the source repository also exists in the worktree
			// checkout, so it already resolves to the worktree's own copy.
			if inside, err := filepath.Rel(fromDir, target); err == nil && inside != ".." && !strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
				continue
			}
			if rel, err := filepath.Rel(toDir, target); err == nil {
				value.Value = filepath.ToSlash(rel)
			} else {
				value.Value = filepath.ToSlash(target)
			}
		}
	}
}

func packageOverrides(rootPath string, packages []domain.PlannedWorktree) overrideSet {
	set := overrideSet{}
	for _, pkg := range packages {
		rel, err := filepath.Rel(rootPath, pkg.Path)
		if err != nil {
			rel = pkg.Path
		}
		set[pkg.Repo.PackageName] = pathOverrideNode(filepath.ToSlash(rel))
	}
	return set
}

func pathOverrideNode(path string) *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "path"},
		{Kind: yaml.ScalarNode, Value: path},
	}}
}

// mergeOverrides combines sets in increasing order of precedence.
func mergeOverrides(sets ...overrideSet) overrideSet {
	merged := overrideSet{}
	for _, set := range sets {
		for name, spec := range set {
			merged[name] = spec
		}
	}
	return merged
}

// formatOverrides renders a deterministic, block-style pubspec_overrides.yaml.
func formatOverrides(set overrideSet) (string, error) {
	if len(set) == 0 {
		return emptyOverrideContent, nil
	}
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)

	section := &yaml.Node{Kind: yaml.MappingNode}
	for _, name := range names {
		spec := set[name]
		clearStyle(spec)
		section.Content = append(section.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: name}, spec)
	}
	doc := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "dependency_overrides"},
		section,
	}}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// clearStyle normalizes flow-style maps (`{path: ../x}`) to block style and
// drops comments so the generated file stays uniform.
func clearStyle(node *yaml.Node) {
	if node.Kind == yaml.MappingNode || node.Kind == yaml.SequenceNode {
		node.Style = 0
	}
	node.HeadComment, node.LineComment, node.FootComment = "", "", ""
	for _, child := range node.Content {
		clearStyle(child)
	}
}

func sourceRepoRootOr(sourceRepoRoot, fallback string) string {
	if sourceRepoRoot == "" {
		return fallback
	}
	return sourceRepoRoot
}
