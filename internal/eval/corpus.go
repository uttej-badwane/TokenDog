package eval

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

//go:embed corpus/*.json
var defaultCorpus embed.FS

// LoadDefault returns the corpus embedded in the binary, so `td eval` works
// without the source tree.
func LoadDefault() ([]Fixture, error) {
	entries, err := defaultCorpus.ReadDir("corpus")
	if err != nil {
		return nil, err
	}
	var fixtures []Fixture
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := defaultCorpus.ReadFile("corpus/" + e.Name())
		if err != nil {
			return nil, err
		}
		f, err := parseFixture(data, e.Name())
		if err != nil {
			return nil, err
		}
		fixtures = append(fixtures, f)
	}
	sortByName(fixtures)
	return fixtures, nil
}

// LoadDir loads a corpus from a directory of *.json files on disk, for users
// curating their own fixtures.
func LoadDir(dir string) ([]Fixture, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var fixtures []Fixture
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		f, err := parseFixture(data, e.Name())
		if err != nil {
			return nil, err
		}
		fixtures = append(fixtures, f)
	}
	if len(fixtures) == 0 {
		return nil, fmt.Errorf("no *.json fixtures found in %s", dir)
	}
	sortByName(fixtures)
	return fixtures, nil
}

func parseFixture(data []byte, file string) (Fixture, error) {
	var f Fixture
	if err := json.Unmarshal(data, &f); err != nil {
		return f, fmt.Errorf("%s: %w", file, err)
	}
	if f.Name == "" {
		f.Name = file
	}
	return f, nil
}

func sortByName(fs []Fixture) {
	sort.Slice(fs, func(i, j int) bool { return fs[i].Name < fs[j].Name })
}
