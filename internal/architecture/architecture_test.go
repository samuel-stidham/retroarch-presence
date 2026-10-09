// Package architecture holds the fitness test for the dependency rule.
package architecture

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const module = "github.com/samuel-stidham/retroarch-presence"

// imports returns every import path used by the non-test Go files in dir.
func imports(t *testing.T, dir string) map[string][]string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no Go files in %s", dir)
	}
	byFile := map[string][]string{}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range parsed.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			byFile[filepath.Base(file)] = append(byFile[filepath.Base(file)], path)
		}
	}
	return byFile
}

func TestDependencyRule(t *testing.T) {
	outward := []string{module + "/internal/infrastructure", module + "/cmd"}
	// The domain does no I/O, so it may not reach for the packages that do.
	ioPackages := []string{"net", "os", "os/exec", "io/fs", "log/slog", "syscall"}

	layers := []struct {
		name      string
		dir       string
		forbidden []string
	}{
		{"domain", "../domain", append(append([]string{module + "/internal/application"}, outward...), ioPackages...)},
		{"application", "../application", append([]string{"net", "os", "os/exec", "syscall"}, outward...)},
	}

	for _, layer := range layers {
		t.Run("given the "+layer.name+" layer, it imports nothing outward", func(t *testing.T) {
			for file, paths := range imports(t, layer.dir) {
				for _, path := range paths {
					for _, banned := range layer.forbidden {
						if path == banned || strings.HasPrefix(path, banned+"/") {
							t.Errorf("%s/%s imports %s", layer.name, file, path)
						}
					}
				}
			}
		})
	}
}

func TestLayersExist(t *testing.T) {
	for _, dir := range []string{"../domain", "../application", "../infrastructure"} {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("layer missing: %v", err)
		}
	}
}
