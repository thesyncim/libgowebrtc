//go:build ignore

// Code generator for C/Go struct layout tests.
//
// Usage: go run main.go
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

type fieldSpec struct {
	CName  string
	GoName string
}

type structSpec struct {
	CName  string
	GoType string
	Fields []fieldSpec
}

type cStructSpec struct {
	Name   string
	Fields []string
}

type goStructSpec struct {
	Name   string
	Fields []string
	Doc    string
}

type genPaths struct {
	ffiDir     string
	shimHeader string
}

var (
	cFieldNameRE = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*$`)
	shimNameRE   = regexp.MustCompile(`\bShim[A-Za-z0-9_]+`)
)

var initialisms = map[string]string{
	"id":   "ID",
	"url":  "URL",
	"urls": "URLs",
	"sdp":  "SDP",
	"rtp":  "RTP",
	"rtcp": "RTCP",
	"rtc":  "RTC",
	"rtt":  "RTT",
	"ice":  "ICE",
	"ssrc": "SSRC",
	"mtu":  "MTU",
	"fps":  "FPS",
	"h264": "H264",
	"vp8":  "VP8",
	"vp9":  "VP9",
	"av1":  "AV1",
	"rid":  "RID",
	"nack": "NACK",
	"pli":  "PLI",
	"fir":  "FIR",
	"qp":   "QP",
	"hw":   "HW",
	"pc":   "PC",
	"dc":   "DC",
}

var specialTokens = map[string]string{
	"mline": "MLine",
}

func main() {
	paths, err := resolvePaths()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving generator paths: %v\n", err)
		os.Exit(1)
	}

	cStructs, err := parseShimStructs(paths.shimHeader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing shim header: %v\n", err)
		os.Exit(1)
	}

	goStructs, err := parseGoStructs(paths.ffiDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing Go structs: %v\n", err)
		os.Exit(1)
	}

	structSpecs, err := buildStructSpecs(cStructs, goStructs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building struct specs: %v\n", err)
		os.Exit(1)
	}

	if err := writeGoFile(filepath.Join(paths.ffiDir, "struct_layout_cgo.go"), generateLayoutGo(structSpecs)); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating struct_layout_cgo.go: %v\n", err)
		os.Exit(1)
	}

	if err := writeGoFile(filepath.Join(paths.ffiDir, "struct_layout_cgo_test.go"), generateLayoutTestGo(structSpecs)); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating struct_layout_cgo_test.go: %v\n", err)
		os.Exit(1)
	}

	if err := writeJSONFile(filepath.Join(paths.ffiDir, "gen", "types.json"), generateTypesJSON(structSpecs)); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating gen/types.json: %v\n", err)
		os.Exit(1)
	}
}

func resolvePaths() (genPaths, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return genPaths{}, errors.New("unable to resolve generator path")
	}
	genDir := filepath.Dir(thisFile)
	ffiDir := filepath.Dir(genDir)
	repoRoot := filepath.Dir(filepath.Dir(ffiDir))
	shimHeader := filepath.Join(repoRoot, "shim", "shim.h")
	return genPaths{ffiDir: ffiDir, shimHeader: shimHeader}, nil
}

func parseShimStructs(path string) (map[string]cStructSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	src := stripCComments(string(data))
	structs := make(map[string]cStructSpec)
	for idx := 0; idx < len(src); {
		pos := strings.Index(src[idx:], "typedef struct")
		if pos == -1 {
			break
		}
		pos += idx

		brace := strings.Index(src[pos:], "{")
		semi := strings.Index(src[pos:], ";")
		if brace == -1 || (semi != -1 && semi < brace) {
			if semi == -1 {
				break
			}
			idx = pos + semi + 1
			continue
		}
		brace += pos

		end, err := findMatchingBrace(src, brace)
		if err != nil {
			return nil, err
		}

		body := src[brace+1 : end]
		name, next := readIdentifier(src, end+1)
		if name == "" {
			return nil, fmt.Errorf("unable to parse struct name near offset %d", end)
		}

		fields := parseCStructFields(body)
		if len(fields) == 0 {
			return nil, fmt.Errorf("no fields found for struct %s", name)
		}
		structs[name] = cStructSpec{Name: name, Fields: fields}
		idx = next
	}

	if len(structs) == 0 {
		return nil, fmt.Errorf("no structs found in %s", path)
	}
	return structs, nil
}

func parseGoStructs(dir string) (map[string]goStructSpec, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	specs := make(map[string]goStructSpec)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		path := filepath.Join(dir, name)
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}

		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.TYPE {
				continue
			}
			for _, spec := range genDecl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				structType, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					continue
				}
				fields := structFieldNames(structType)
				doc := docText(typeSpec.Doc, genDecl.Doc)
				specs[typeSpec.Name.Name] = goStructSpec{
					Name:   typeSpec.Name.Name,
					Fields: fields,
					Doc:    doc,
				}
			}
		}
	}

	if len(specs) == 0 {
		return nil, fmt.Errorf("no Go structs found in %s", dir)
	}
	return specs, nil
}

func buildStructSpecs(cStructs map[string]cStructSpec, goStructs map[string]goStructSpec) ([]structSpec, error) {
	mapped := make(map[string]structSpec)
	for _, goSpec := range goStructs {
		cName, err := matchCStructName(goSpec, cStructs)
		if err != nil {
			return nil, err
		}
		if cName == "" {
			continue
		}

		cSpec := cStructs[cName]
		spec, err := mapStructFields(cSpec, goSpec)
		if err != nil {
			return nil, err
		}
		mapped[cName] = spec
	}

	var missing []string
	for name := range cStructs {
		if _, ok := mapped[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("missing Go struct matches for: %s", strings.Join(missing, ", "))
	}

	names := make([]string, 0, len(mapped))
	for name := range mapped {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]structSpec, 0, len(names))
	for _, name := range names {
		out = append(out, mapped[name])
	}
	return out, nil
}

func matchCStructName(goSpec goStructSpec, cStructs map[string]cStructSpec) (string, error) {
	if shimName := shimNameFromDoc(goSpec.Doc); shimName != "" {
		if _, ok := cStructs[shimName]; !ok {
			return "", fmt.Errorf("struct %s references %s but it is not in shim.h", goSpec.Name, shimName)
		}
		return shimName, nil
	}

	if _, ok := cStructs[goSpec.Name]; ok {
		return goSpec.Name, nil
	}

	if strings.HasPrefix(goSpec.Name, "shim") {
		candidate := "Shim" + goSpec.Name[len("shim"):]
		if _, ok := cStructs[candidate]; ok {
			return candidate, nil
		}
	}

	return "", nil
}

func mapStructFields(cSpec cStructSpec, goSpec goStructSpec) (structSpec, error) {
	goFields := make(map[string]struct{}, len(goSpec.Fields))
	for _, name := range goSpec.Fields {
		goFields[name] = struct{}{}
	}

	matched := make(map[string]struct{}, len(goSpec.Fields))
	fields := make([]fieldSpec, 0, len(cSpec.Fields))
	for _, cField := range cSpec.Fields {
		goName, ok := matchGoField(cField, goFields)
		if !ok {
			return structSpec{}, fmt.Errorf("%s: no Go field matches C field %q", cSpec.Name, cField)
		}
		fields = append(fields, fieldSpec{CName: cField, GoName: goName})
		matched[goName] = struct{}{}
	}

	var extra []string
	for name := range goFields {
		if _, ok := matched[name]; !ok {
			extra = append(extra, name)
		}
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		return structSpec{}, fmt.Errorf("%s: Go fields not present in C: %s", cSpec.Name, strings.Join(extra, ", "))
	}

	return structSpec{CName: cSpec.Name, GoType: goSpec.Name, Fields: fields}, nil
}

func matchGoField(cField string, goFields map[string]struct{}) (string, bool) {
	for _, candidate := range cFieldCandidates(cField) {
		if _, ok := goFields[candidate]; ok {
			return candidate, true
		}
	}
	return "", false
}

func cFieldCandidates(cField string) []string {
	parts := strings.Split(cField, "_")
	candInit := tokensToGo(parts, true)
	candTitle := tokensToGo(parts, false)

	candidates := []string{
		candInit,
		lowerFirst(candInit),
		candTitle,
		lowerFirst(candTitle),
	}
	return uniqueStrings(candidates)
}

func tokensToGo(tokens []string, useInitialisms bool) string {
	var b strings.Builder
	for _, token := range tokens {
		lower := strings.ToLower(token)
		if useInitialisms {
			if mapped, ok := specialTokens[lower]; ok {
				b.WriteString(mapped)
				continue
			}
			if mapped, ok := initialisms[lower]; ok {
				b.WriteString(mapped)
				continue
			}
		}
		b.WriteString(titleCase(lower))
	}
	return b.String()
}

func titleCase(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func lowerFirst(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, item := range in {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func shimNameFromDoc(doc string) string {
	if doc == "" {
		return ""
	}
	if !strings.Contains(strings.ToLower(doc), "matches") {
		return ""
	}
	return shimNameRE.FindString(doc)
}

func docText(typeDoc, declDoc *ast.CommentGroup) string {
	if typeDoc != nil {
		return strings.TrimSpace(typeDoc.Text())
	}
	if declDoc != nil {
		return strings.TrimSpace(declDoc.Text())
	}
	return ""
}

func structFieldNames(st *ast.StructType) []string {
	fields := make([]string, 0, len(st.Fields.List))
	for _, field := range st.Fields.List {
		if len(field.Names) == 0 {
			continue
		}
		for _, name := range field.Names {
			if name.Name == "_" {
				continue
			}
			fields = append(fields, name.Name)
		}
	}
	return fields
}

func stripCComments(src string) string {
	var out strings.Builder
	inLine := false
	inBlock := false

	for i := 0; i < len(src); i++ {
		if inLine {
			if src[i] == '\n' {
				inLine = false
				out.WriteByte(src[i])
			}
			continue
		}
		if inBlock {
			if i+1 < len(src) && src[i] == '*' && src[i+1] == '/' {
				inBlock = false
				i++
			}
			continue
		}
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '/' {
			inLine = true
			i++
			continue
		}
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '*' {
			inBlock = true
			i++
			continue
		}
		out.WriteByte(src[i])
	}

	return out.String()
}

func findMatchingBrace(src string, start int) (int, error) {
	depth := 0
	for i := start; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, nil
			}
		}
	}
	return 0, errors.New("unbalanced braces in shim header")
}

func readIdentifier(src string, start int) (string, int) {
	i := start
	for i < len(src) && isSpace(src[i]) {
		i++
	}
	j := i
	for j < len(src) && isIdentChar(src[j]) {
		j++
	}
	return src[i:j], j
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\n' || b == '\t' || b == '\r'
}

func isIdentChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

func parseCStructFields(body string) []string {
	parts := strings.Split(body, ";")
	fields := make([]string, 0, len(parts))
	for _, part := range parts {
		name, ok := parseCFieldName(part)
		if !ok {
			continue
		}
		fields = append(fields, name)
	}
	return fields
}

func parseCFieldName(decl string) (string, bool) {
	decl = strings.TrimSpace(decl)
	if decl == "" {
		return "", false
	}
	if strings.HasPrefix(decl, "#") {
		return "", false
	}
	if idx := strings.Index(decl, "["); idx != -1 {
		decl = decl[:idx]
	}
	decl = strings.TrimSpace(decl)
	name := cFieldNameRE.FindString(decl)
	if name == "" {
		return "", false
	}
	return name, true
}

func generateLayoutGo(specs []structSpec) []byte {
	var buf bytes.Buffer

	buf.WriteString(`// Code generated by go generate; DO NOT EDIT.

//go:build ffigo_cgo

package ffi

/*
#cgo CFLAGS: -I${SRCDIR}/../../shim
#include "shim.h"
*/
import "C"

import "unsafe"

type cStructLayout struct {
	size    uintptr
	offsets map[string]uintptr
}

`)

	for _, spec := range specs {
		fmt.Fprintf(&buf, "func %s() cStructLayout {\n", layoutFuncName(spec.CName))
		fmt.Fprintf(&buf, "\tvar cCfg C.%s\n", spec.CName)
		buf.WriteString("\treturn cStructLayout{\n")
		buf.WriteString("\t\tsize: unsafe.Sizeof(cCfg),\n")
		buf.WriteString("\t\toffsets: map[string]uintptr{\n")
		for _, field := range spec.Fields {
			fmt.Fprintf(&buf, "\t\t\t%q: unsafe.Offsetof(cCfg.%s),\n", field.GoName, cgoFieldName(field.CName))
		}
		buf.WriteString("\t\t},\n")
		buf.WriteString("\t}\n")
		buf.WriteString("}\n\n")
	}

	return buf.Bytes()
}

func generateLayoutTestGo(specs []structSpec) []byte {
	var buf bytes.Buffer

	buf.WriteString(`// Code generated by go generate; DO NOT EDIT.

//go:build ffigo_cgo

package ffi

import (
	"testing"
	"unsafe"
)

`)

	buf.WriteString("// TestShimStructLayoutCgo compares Go struct layouts against the C shim headers.\n")
	buf.WriteString("func TestShimStructLayoutCgo(t *testing.T) {\n")
	for _, spec := range specs {
		fmt.Fprintf(&buf, "\tt.Run(%q, func(t *testing.T) {\n", spec.CName)
		fmt.Fprintf(&buf, "\t\tvar goCfg %s\n", spec.GoType)
		fmt.Fprintf(&buf, "\t\tlayout := %s()\n", layoutFuncName(spec.CName))
		fmt.Fprintf(&buf, "\t\tcheckSizeEqual(t, %q, unsafe.Sizeof(goCfg), layout.size)\n", spec.CName)
		for _, field := range spec.Fields {
			fmt.Fprintf(&buf, "\t\tcheckOffsetEqual(t, %q, unsafe.Offsetof(goCfg.%s), layout.offsets[%q])\n",
				fmt.Sprintf("%s.%s", spec.CName, field.GoName), field.GoName, field.GoName)
		}
		buf.WriteString("\t})\n\n")
	}
	buf.WriteString("}\n\n")
	buf.WriteString("func checkSizeEqual(t *testing.T, name string, goSize, cSize uintptr) {\n")
	buf.WriteString("\tt.Helper()\n")
	buf.WriteString("\tif goSize != cSize {\n")
	buf.WriteString("\t\tt.Errorf(\"%s size = %d, want %d\", name, goSize, cSize)\n")
	buf.WriteString("\t}\n")
	buf.WriteString("}\n\n")
	buf.WriteString("func checkOffsetEqual(t *testing.T, name string, goOffset, cOffset uintptr) {\n")
	buf.WriteString("\tt.Helper()\n")
	buf.WriteString("\tif goOffset != cOffset {\n")
	buf.WriteString("\t\tt.Errorf(\"%s offset = %d, want %d\", name, goOffset, cOffset)\n")
	buf.WriteString("\t}\n")
	buf.WriteString("}\n")

	return buf.Bytes()
}

func layoutFuncName(cName string) string {
	return "c" + cName + "Layout"
}

func cgoFieldName(cName string) string {
	if token.Lookup(cName) != token.IDENT {
		return "_" + cName
	}
	return cName
}

type typesFile struct {
	GeneratedFrom string       `json:"generated_from"`
	Structs       []typesEntry `json:"structs"`
}

type typesEntry struct {
	CName  string       `json:"c_name"`
	GoName string       `json:"go_name"`
	Fields []typesField `json:"fields"`
}

type typesField struct {
	CName  string `json:"c_name"`
	GoName string `json:"go_name"`
}

func generateTypesJSON(specs []structSpec) []byte {
	out := typesFile{
		GeneratedFrom: filepath.ToSlash(filepath.Join("shim", "shim.h")),
		Structs:       make([]typesEntry, 0, len(specs)),
	}
	for _, spec := range specs {
		entry := typesEntry{
			CName:  spec.CName,
			GoName: spec.GoType,
			Fields: make([]typesField, 0, len(spec.Fields)),
		}
		for _, field := range spec.Fields {
			entry.Fields = append(entry.Fields, typesField{
				CName:  field.CName,
				GoName: field.GoName,
			})
		}
		out.Structs = append(out.Structs, entry)
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		panic(err)
	}
	return append(data, '\n')
}

func writeGoFile(path string, data []byte) error {
	formatted, err := format.Source(data)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, formatted, 0644)
}

func writeJSONFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
