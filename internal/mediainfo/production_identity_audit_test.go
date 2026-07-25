package mediainfo

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

var namespacedSampleIDPattern = regexp.MustCompile(`\b[PB]:S[0-9]{3}\b`)

var forbiddenProductionLiterals = []string{
	"nickelodeon - generic halloween promo",
	"disney channel - evermoor behind the scenes",
	"reigen ohatsu",
	"super bowl halftime show 2005 paul mccartney",
	"30f2828a45a1895b033c3cd7784581033327e7b393033c55f4a03bb15cab0d89",
	"4d3f50cd7369a2e4fe2672d982d44a84b220454da10746b8cef43451ee0d3acc",
	"953d36f54ab9c645a486e6435aae9f25beee687622967dce774880596e21f448",
	"11870af51d3d0855bb6b7040eccaccda51b31208ff25712b3a7089afa8950241",
	`go-mediainfo\parity\references`,
	`go-mediainfo/parity/references`,
	`go-mediainfo\parity\bdmv-checks`,
	`go-mediainfo/parity/bdmv-checks`,
}

func TestProductionSourceHasNoCorpusIdentitySelectors(t *testing.T) {
	t.Parallel()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate audit source")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	issues, err := auditProductionIdentityTree(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) > 0 {
		t.Fatalf("production identity audit:\n%s", strings.Join(issues, "\n"))
	}
}

func TestProductionIdentityAuditCoversRepoPackages(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	paths := []string{
		"root.go",
		filepath.Join("internal", "cli", "cli.go"),
		filepath.Join("internal", "mediainfo", "analyze.go"),
	}
	for _, name := range paths {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package sample\nconst corpusID = \"P:S021\"\n"), 0o644); err != nil { //nolint:gosec // test fixture
			t.Fatal(err)
		}
	}

	issues, err := auditProductionIdentityTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != len(paths) {
		t.Fatalf("issues = %v, want one for each production package", issues)
	}
	for _, name := range paths {
		want := filepath.ToSlash(name) + ": corpus sample ID"
		if !strings.Contains(strings.Join(issues, "\n"), want) {
			t.Errorf("missing audit issue for %s: %v", name, issues)
		}
	}
}

func TestProductionIdentityAuditDistinguishesReportingFromSelectors(t *testing.T) {
	t.Parallel()

	allowed := []byte(`package sample
func report(uniqueID string, fields map[string]string) string {
	fields["UniqueID"] = uniqueID
	return uniqueID
}`)
	if issues := auditProductionIdentitySource("allowed.go", allowed); len(issues) != 0 {
		t.Fatalf("identity reporting rejected: %v", issues)
	}

	selector := []byte(`package sample
func project(structured map[string]string) string {
	if structured["UniqueID"] == "123" {
		return "reference override"
	}
	return ""
}`)
	if issues := auditProductionIdentitySource("selector.go", selector); len(issues) != 1 {
		t.Fatalf("identity selector issues = %v, want one", issues)
	}

	corpusLiteral := []byte(`package sample
const input = "P:S021"
`)
	if issues := auditProductionIdentitySource("literal.go", corpusLiteral); len(issues) != 1 {
		t.Fatalf("corpus literal issues = %v, want one", issues)
	}
}

func auditProductionIdentityTree(root string) ([]string, error) {
	var issues []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path == root {
				return nil
			}
			name := entry.Name()
			if strings.HasPrefix(name, ".") || name == "testdata" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		name, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		issues = append(issues, auditProductionIdentitySource(filepath.ToSlash(name), source)...)
		return nil
	})
	return issues, err
}

func auditProductionIdentitySource(name string, source []byte) []string {
	lower := strings.ToLower(string(source))
	var issues []string
	if match := namespacedSampleIDPattern.FindString(string(source)); match != "" {
		issues = append(issues, fmt.Sprintf("%s: corpus sample ID %q", name, match))
	}
	for _, literal := range forbiddenProductionLiterals {
		if strings.Contains(lower, literal) {
			issues = append(issues, fmt.Sprintf("%s: corpus/reference literal %q", name, literal))
		}
	}

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, name, source, 0)
	if err != nil {
		return append(issues, fmt.Sprintf("%s: parse: %v", name, err))
	}
	ast.Inspect(file, func(node ast.Node) bool {
		var condition ast.Expr
		switch typed := node.(type) {
		case *ast.IfStmt:
			condition = typed.Cond
		case *ast.SwitchStmt:
			condition = typed.Tag
		}
		if condition == nil {
			return true
		}
		var rendered bytes.Buffer
		if err := format.Node(&rendered, fileSet, condition); err != nil {
			issues = append(issues, fmt.Sprintf("%s: render condition: %v", name, err))
			return true
		}
		text := rendered.String()
		if strings.Contains(text, `["UniqueID"]`) || strings.Contains(text, `["TrackUID"]`) || strings.Contains(text, `["SegmentUID"]`) {
			position := fileSet.Position(condition.Pos())
			issues = append(issues, fmt.Sprintf("%s:%d: identity metadata controls output: %s", name, position.Line, text))
		}
		return true
	})
	return issues
}
