package app

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var defaultRootFilePatterns = []string{".env", ".env.*"}

func mergeRootFilePatterns(extra []string) []string {
	patterns := append([]string{}, defaultRootFilePatterns...)
	for _, item := range extra {
		token := strings.TrimSpace(item)
		if token == "" {
			continue
		}
		patterns = append(patterns, token)
	}
	return dedupStringsPreservingOrder(patterns)
}

func resolveRootFilesToCopy(sourceRoot string, patterns []string) []string {
	resolved := []string{}
	seen := map[string]struct{}{}

	for _, pattern := range patterns {
		if strings.ContainsAny(pattern, "*?[]") {
			matches, _ := filepath.Glob(filepath.Join(sourceRoot, pattern))
			sort.Strings(matches)
			for _, match := range matches {
				info, err := os.Stat(match)
				if err != nil || info.IsDir() {
					continue
				}
				if _, ok := seen[match]; ok {
					continue
				}
				seen[match] = struct{}{}
				resolved = append(resolved, match)
			}
			continue
		}

		target := filepath.Join(sourceRoot, pattern)
		info, err := os.Stat(target)
		if err != nil || info.IsDir() {
			continue
		}
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		resolved = append(resolved, target)
	}

	return resolved
}

func copyRootFiles(sourceRoot, targetRoot string, patterns []string) error {
	files := resolveRootFilesToCopy(sourceRoot, patterns)
	for _, sourcePath := range files {
		destPath := filepath.Join(targetRoot, filepath.Base(sourcePath))
		if err := copyFile(sourcePath, destPath); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(sourcePath, destinationPath string) error {
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		return err
	}

	src, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return err
	}

	dst, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}
