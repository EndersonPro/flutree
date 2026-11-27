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

// overrideExcludePattern keeps the generated file out of commits without
// touching the tracked .gitignore; anchored so it only matches the root file.
const overrideExcludePattern = "/" + overrideFileName

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
//
// Plain path dependencies are carried as well (see deriveGraphOverrides) at the
// lowest precedence, so the whole graph resolves from the worktrees.
func writeOverrideFile(overridePath, worktreePath, sourceRepoRoot string, packages []domain.PlannedWorktree) error {
	content, _, err := renderOverrides(overrideTarget{
		worktree:     worktreePath,
		source:       sourceRepoRootOr(sourceRepoRoot, worktreePath),
		overridePath: overridePath,
		packages:     packages,
	})
	if err != nil {
		return err
	}
	if current, err := os.ReadFile(overridePath); err == nil && string(current) == content {
		return nil
	}
	return os.WriteFile(overridePath, []byte(content), 0o644)
}

// overrideTarget describes one pubspec_overrides.yaml to compute: the root's or
// a package's. Both are computed from the point of view of their own worktree
// and source repository, over the packages that belong to their graph.
type overrideTarget struct {
	worktree     string
	source       string
	overridePath string
	// packages are the selected package worktrees whose graph this file covers.
	packages []domain.PlannedWorktree
	// self is the package name of the target when it is a package; it never gets
	// an entry for itself.
	self string
	// fromSource reads the pubspecs from the source checkouts, for the dry plan
	// before any worktree exists. It also tolerates unreadable pubspecs.
	fromSource bool
}

// renderOverrides computes the content of a pubspec_overrides.yaml and the
// number of entries it carries (zero means nothing is needed).
func renderOverrides(t overrideTarget) (string, int, error) {
	pubspecDir := t.worktree
	if t.fromSource {
		pubspecDir = t.source
	}
	pubspecOverrides, err := readOverridesSection(filepath.Join(pubspecDir, "pubspec.yaml"))
	if err != nil {
		if !t.fromSource {
			return "", 0, err
		}
		pubspecOverrides = overrideSet{}
	}
	rebasePathOverrides(pubspecOverrides, t.source, t.worktree)

	existing := overrideSet{}
	if !t.fromSource {
		existing, err = readOverridesSection(t.overridePath)
		if err != nil {
			return "", 0, err
		}
	}

	derived, err := deriveGraphOverrides(t.worktree, t.source, t.packages, t.fromSource)
	if err != nil {
		if !t.fromSource {
			return "", 0, err
		}
		derived = overrideSet{}
	}
	dropStaleGenerated(existing, derived, t.worktree, t.packages)
	merged := mergeOverrides(derived, pubspecOverrides, existing, packageOverrides(t.worktree, t.packages))
	if t.self != "" {
		delete(merged, t.self)
	}
	content, err := formatOverrides(merged)
	if err != nil {
		if t.fromSource {
			return emptyOverrideContent, 0, nil
		}
		return "", 0, err
	}
	return content, len(merged), nil
}

// dropStaleGenerated removes from existing the entries a previous run generated
// and the current graph now maps elsewhere, so attaching a repository refreshes
// them. The file carries no marker, so an entry counts as generated only when
// the new derivation has a different value for the same package and the
// existing path still points into the source checkout of a selected repository:
// that is the location flutree itself produced before the repository had a
// worktree. Anything else (a user's own path, version, git or hosted spec)
// stays and keeps precedence over the derived values.
func dropStaleGenerated(existing, derived overrideSet, worktreePath string, packages []domain.PlannedWorktree) {
	for name, spec := range existing {
		fresh, ok := derived[name]
		if !ok {
			continue
		}
		have, hasPath := pathSpec(spec)
		want, _ := pathSpec(fresh)
		if !hasPath || have == want {
			continue
		}
		target := have
		if !filepath.IsAbs(target) {
			target = filepath.Join(worktreePath, filepath.FromSlash(have))
		}
		target = filepath.Clean(target)
		for _, pkg := range packages {
			if rel, err := filepath.Rel(pkg.Repo.RepoRoot, target); err == nil && !isOutside(rel) {
				delete(existing, name)
				break
			}
		}
	}
}

// buildOverrideContent renders the override file shown in the dry plan, before
// the root worktree exists, from the source repository's pubspec.yaml.
func buildOverrideContent(root domain.PlannedWorktree, packages []domain.PlannedWorktree) string {
	content, _, _ := renderOverrides(overrideTarget{
		worktree:   root.Path,
		source:     root.Repo.RepoRoot,
		packages:   packages,
		fromSource: true,
	})
	return content
}

// readOverridesSection returns the dependency_overrides of a pubspec-style
// file. A missing file or section yields an empty set.
func readOverridesSection(path string) (overrideSet, error) {
	return readPubspecSection(path, "dependency_overrides")
}

// readPubspecSection returns the entries of one top-level mapping section of a
// pubspec-style file. A missing file or section yields an empty set.
func readPubspecSection(path, section string) (overrideSet, error) {
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
		if top.Content[i].Value != section {
			continue
		}
		body := top.Content[i+1]
		if body.Kind != yaml.MappingNode {
			return set, nil
		}
		for j := 0; j+1 < len(body.Content); j += 2 {
			set[body.Content[j].Value] = body.Content[j+1]
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

// graphRepo pairs a repository's source checkout with the worktree flutree
// manages for it.
type graphRepo struct {
	source   string
	worktree string
}

// deriveGraphOverrides turns the relative `path:` dependencies of the root and
// of every selected package into overrides for the root's
// pubspec_overrides.yaml. Overrides of the root package apply to the whole
// resolution graph, so this is the one place that can repair a path that is
// only valid from the source checkout.
//
// Each path is resolved against the declaring repo's source root. A target
// inside a selected repo maps to the same location in that repo's worktree;
// any other target stays at its source location. Entries that already resolve
// to the same directory from the declaring worktree need no override. The root
// pubspec wins over packages and packages are visited by name, so the result is
// deterministic. fromSource reads the pubspecs from the source checkouts, for
// the dry plan before any worktree exists.
func deriveGraphOverrides(rootWorktree, rootSource string, packages []domain.PlannedWorktree, fromSource bool) (overrideSet, error) {
	repos := []graphRepo{{source: rootSource, worktree: rootWorktree}}
	ordered := append([]domain.PlannedWorktree(nil), packages...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Repo.PackageName < ordered[j].Repo.PackageName })
	for _, pkg := range ordered {
		repos = append(repos, graphRepo{source: pkg.Repo.RepoRoot, worktree: pkg.Path})
	}

	derived := overrideSet{}
	claimed := map[string]bool{}
	for i, repo := range repos {
		pubspecDir := repo.worktree
		if fromSource {
			pubspecDir = repo.source
		}
		sections := []string{"dependencies", "dev_dependencies"}
		if i > 0 {
			// Only the root package's own overrides apply; a package's are
			// ignored by pub, so they are carried as plain path dependencies.
			sections = append(sections, "dependency_overrides")
		}
		for _, section := range sections {
			deps, err := readPubspecSection(filepath.Join(pubspecDir, "pubspec.yaml"), section)
			if err != nil {
				return nil, err
			}
			for _, name := range sortedNames(deps) {
				rel, ok := pathSpec(deps[name])
				if !ok || claimed[name] {
					continue
				}
				claimed[name] = true
				target, mapped := mapGraphTarget(repo, rel, repos)
				original := rel
				if !filepath.IsAbs(rel) {
					original = filepath.Join(repo.worktree, filepath.FromSlash(rel))
				}
				if target == filepath.Clean(original) {
					continue
				}
				if mapped {
					if relTarget, err := filepath.Rel(rootWorktree, target); err == nil {
						target = relTarget
					}
				}
				derived[name] = pathOverrideNode(filepath.ToSlash(target))
			}
		}
	}
	return derived, nil
}

// mapGraphTarget resolves a declared path against the declaring repo's source
// root and maps it into the worktree of the selected repo that contains it, or
// keeps the source location when no selected repo does.
func mapGraphTarget(declaring graphRepo, rel string, repos []graphRepo) (string, bool) {
	if filepath.IsAbs(rel) {
		return filepath.Clean(rel), false
	}
	target := filepath.Join(declaring.source, filepath.FromSlash(rel))
	best, bestRel := -1, ""
	for i, repo := range repos {
		inside, err := filepath.Rel(repo.source, target)
		if err != nil || isOutside(inside) {
			continue
		}
		if best < 0 || len(repo.source) > len(repos[best].source) {
			best, bestRel = i, inside
		}
	}
	if best < 0 {
		return target, false
	}
	return filepath.Join(repos[best].worktree, bestRel), true
}

func isOutside(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// pathSpec returns the relative or absolute directory of a `path:` dependency.
func pathSpec(spec *yaml.Node) (string, bool) {
	if spec == nil || spec.Kind != yaml.MappingNode {
		return "", false
	}
	for i := 0; i+1 < len(spec.Content); i += 2 {
		value := spec.Content[i+1]
		if spec.Content[i].Value == "path" && value.Kind == yaml.ScalarNode && value.Value != "" {
			return value.Value, true
		}
	}
	return "", false
}

func sortedNames(set overrideSet) []string {
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Package override files.
//
// `flutree pubget` runs pub get inside every package worktree, and pub only
// honours the overrides of the package it resolves, so a selected package that
// depends on another selected package needs its own pubspec_overrides.yaml
// pointing at that package's worktree. It is computed with the same rules as the
// root file, from the point of view of the package worktree and its source repo.

const (
	packageOverrideWritten   = "written"
	packageOverrideSkipped   = "skipped"
	packageOverrideUnchanged = "unchanged"
)

// packageOverridePlan is the file one package worktree needs.
type packageOverridePlan struct {
	pkg     domain.PlannedWorktree
	path    string
	content string
	// skipNote is set when flutree deliberately writes nothing for this package.
	skipNote string
	// needed is false when the package has no entry to override.
	needed bool
}

// packageUsesPubWorkspace reports whether the pubspec in dir opts into Dart pub
// workspaces (`resolution: workspace`).
func packageUsesPubWorkspace(dir string) bool {
	content, err := os.ReadFile(filepath.Join(dir, "pubspec.yaml"))
	if err != nil {
		return false
	}
	var doc struct {
		Resolution string `yaml:"resolution"`
	}
	if yaml.Unmarshal(content, &doc) != nil {
		return false
	}
	return strings.TrimSpace(doc.Resolution) == "workspace"
}

// packageGraphMembers returns the selected packages pkg depends on, directly or
// through other selected packages, ordered by package name. The package's own
// dependencies and dev_dependencies seed the search; selected packages it
// reaches contribute their dependencies. Specs of any shape (path, git, hosted,
// version) count: only the package name matters.
func packageGraphMembers(pkg domain.PlannedWorktree, all []domain.PlannedWorktree, fromSource bool) ([]domain.PlannedWorktree, error) {
	byName := map[string]domain.PlannedWorktree{}
	for _, other := range all {
		if other.Repo.PackageName != "" && other.Repo.PackageName != pkg.Repo.PackageName {
			byName[other.Repo.PackageName] = other
		}
	}
	pubspecOf := func(p domain.PlannedWorktree) string {
		dir := p.Path
		if fromSource {
			dir = p.Repo.RepoRoot
		}
		return filepath.Join(dir, "pubspec.yaml")
	}

	var queue []string
	read := func(p domain.PlannedWorktree, sections ...string) error {
		for _, section := range sections {
			deps, err := readPubspecSection(pubspecOf(p), section)
			if err != nil {
				return err
			}
			queue = append(queue, sortedNames(deps)...)
		}
		return nil
	}
	if err := read(pkg, "dependencies", "dev_dependencies"); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	members := []domain.PlannedWorktree{}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		member, ok := byName[name]
		if !ok || seen[name] {
			continue
		}
		seen[name] = true
		members = append(members, member)
		if err := read(member, "dependencies"); err != nil {
			return nil, err
		}
	}
	sort.Slice(members, func(i, j int) bool { return members[i].Repo.PackageName < members[j].Repo.PackageName })
	return members, nil
}

// planPackageOverride computes the override file of one package worktree.
func planPackageOverride(pkg domain.PlannedWorktree, all []domain.PlannedWorktree, fromSource bool) (packageOverridePlan, error) {
	plan := packageOverridePlan{pkg: pkg, path: filepath.Join(pkg.Path, overrideFileName)}
	pubspecDir := pkg.Path
	if fromSource {
		pubspecDir = pkg.Repo.RepoRoot
	}
	if packageUsesPubWorkspace(pubspecDir) {
		plan.skipNote = "Uses Dart pub workspaces (resolution: workspace); flutree does not write overrides for it."
		return plan, nil
	}
	members, err := packageGraphMembers(pkg, all, fromSource)
	if err != nil {
		return plan, err
	}
	content, entries, err := renderOverrides(overrideTarget{
		worktree:     pkg.Path,
		source:       pkg.Repo.RepoRoot,
		overridePath: plan.path,
		packages:     members,
		self:         pkg.Repo.PackageName,
		fromSource:   fromSource,
	})
	if err != nil {
		return plan, err
	}
	plan.content, plan.needed = content, entries > 0
	return plan, nil
}

// buildPackageOverridePlans renders, for the dry plan, the file every package
// worktree will get. Packages that need nothing are left out.
func buildPackageOverridePlans(packages []domain.PlannedWorktree) []domain.PackageOverridePlan {
	out := []domain.PackageOverridePlan{}
	for _, pkg := range packages {
		plan, err := planPackageOverride(pkg, packages, true)
		if err != nil || (!plan.needed && plan.skipNote == "") {
			continue
		}
		entry := domain.PackageOverridePlan{Repo: pkg.Repo.Name, Path: plan.path, Note: plan.skipNote}
		if plan.needed {
			entry.Content = plan.content
		}
		out = append(out, entry)
	}
	return out
}

// writePackageOverrides writes the override file of every package worktree and
// excludes it locally. A package that needs no entry gets no file.
func writePackageOverrides(git interface{ EnsureLocalExclude(string, string) error }, packages []domain.PlannedWorktree) ([]domain.PackageOverrideResult, error) {
	results := []domain.PackageOverrideResult{}
	for _, pkg := range packages {
		plan, err := planPackageOverride(pkg, packages, false)
		if err != nil {
			return results, fmt.Errorf("%s: %w", pkg.Repo.Name, err)
		}
		result := domain.PackageOverrideResult{Repo: pkg.Repo.Name, Path: plan.path}
		switch {
		case plan.skipNote != "":
			result.Status, result.Note = packageOverrideSkipped, plan.skipNote
		case !plan.needed:
			continue
		default:
			if current, err := os.ReadFile(plan.path); err == nil && string(current) == plan.content {
				result.Status = packageOverrideUnchanged
			} else if err := os.WriteFile(plan.path, []byte(plan.content), 0o644); err != nil {
				return results, fmt.Errorf("%s: %w", pkg.Repo.Name, err)
			} else {
				result.Status = packageOverrideWritten
			}
			if err := git.EnsureLocalExclude(pkg.Path, overrideExcludePattern); err != nil {
				return results, fmt.Errorf("%s: %w", pkg.Repo.Name, err)
			}
		}
		results = append(results, result)
	}
	return results, nil
}
