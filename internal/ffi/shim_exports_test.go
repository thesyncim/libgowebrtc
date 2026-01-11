package ffi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"
)

type shimFuncDefs struct {
	Functions []struct {
		CName string `json:"c_name"`
	} `json:"functions"`
}

// TestShimExportsCovered ensures every SHIM_EXPORT in shim.h is present in funcs.json.
func TestShimExportsCovered(t *testing.T) {
	shimPath := filepath.Join("..", "..", "shim", "shim.h")
	shimData, err := os.ReadFile(shimPath)
	if err != nil {
		t.Fatalf("read shim header: %v", err)
	}

	exportRe := regexp.MustCompile(`(?m)^[ \t]*SHIM_EXPORT\s+[^;]*?\b([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	matches := exportRe.FindAllSubmatch(shimData, -1)
	if len(matches) == 0 {
		t.Fatal("no SHIM_EXPORT functions found in shim.h")
	}

	exported := make(map[string]struct{}, len(matches))
	for _, m := range matches {
		exported[string(m[1])] = struct{}{}
	}

	funcsPath := filepath.Join("gen", "funcs.json")
	funcsData, err := os.ReadFile(funcsPath)
	if err != nil {
		t.Fatalf("read funcs.json: %v", err)
	}

	var defs shimFuncDefs
	if err := json.Unmarshal(funcsData, &defs); err != nil {
		t.Fatalf("parse funcs.json: %v", err)
	}

	bound := make(map[string]struct{}, len(defs.Functions))
	for _, fn := range defs.Functions {
		bound[fn.CName] = struct{}{}
	}

	missing := diffShimNames(exported, bound)
	extra := diffShimNames(bound, exported)
	if len(missing) > 0 || len(extra) > 0 {
		t.Fatalf("shim exports mismatch: missing=%v extra=%v", missing, extra)
	}
}

func diffShimNames(a, b map[string]struct{}) []string {
	out := make([]string, 0, len(a))
	for name := range a {
		if _, ok := b[name]; !ok {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}
