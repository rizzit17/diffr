package astgraph

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"diffr/internal/diffengine"
)

// Builtin Go functions that should not be treated as graph nodes.
var builtinFuncs = map[string]bool{
	"append": true, "cap": true, "close": true, "complex": true, "copy": true,
	"delete": true, "imag": true, "len": true, "make": true, "new": true,
	"panic": true, "print": true, "println": true, "real": true, "recover": true,
}

// FuncInfo contains metadata about a declared Go function or method.
type FuncInfo struct {
	ID        string // e.g. "math.Add" or "math.(*Calculator).Compute"
	Package   string // e.g. "math"
	PkgDir    string // relative directory, e.g. "pkg/math"
	File      string // relative file path, e.g. "pkg/math/calc.go"
	Name      string // function/method name, e.g. "Add" or "Compute"
	Receiver  string // receiver type if method, e.g. "*Calculator", or ""
	StartLine int    // 1-based start line
	EndLine   int    // 1-based end line
	IsTest    bool   // true if function is Test* in a _test.go file
}

// Graph holds the AST-derived call graph and reverse call graph.
type Graph struct {
	RepoDir      string
	Functions    map[string]*FuncInfo // ID -> FuncInfo
	CallGraph    map[string][]string  // callerID -> []calleeID
	ReverseGraph map[string][]string  // calleeID -> []callerID
	AllTests     []string             // list of all test IDs in repo
}

// Build walks repoDir, parses all Go files into ASTs, and builds the call graph.
func Build(repoDir string) (*Graph, error) {
	g := &Graph{
		RepoDir:      repoDir,
		Functions:    make(map[string]*FuncInfo),
		CallGraph:    make(map[string][]string),
		ReverseGraph: make(map[string][]string),
		AllTests:     []string{},
	}

	fset := token.NewFileSet()
	type fileAST struct {
		relPath string
		pkgDir  string
		file    *ast.File
		imports map[string]string // alias/pkgName -> importPath
	}

	var parsedFiles []fileAST
	methodsByName := make(map[string][]*FuncInfo) // methodName -> []*FuncInfo

	// 1. Walk repo and parse all .go files (excluding vendor, git, hidden dirs)
	err := filepath.WalkDir(repoDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if name == "vendor" || name == ".git" || name == "node_modules" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(name, ".go") {
			return nil
		}

		relPath, err := filepath.Rel(repoDir, path)
		if err != nil {
			relPath = path
		}
		relPath = filepath.ToSlash(relPath)
		pkgDir := filepath.ToSlash(filepath.Dir(relPath))

		fileNode, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			// Skip unparseable files rather than failing the entire run
			return nil
		}

		importMap := make(map[string]string)
		for _, imp := range fileNode.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			var alias string
			if imp.Name != nil {
				alias = imp.Name.Name
			} else {
				parts := strings.Split(importPath, "/")
				alias = parts[len(parts)-1]
			}
			importMap[alias] = importPath
		}

		parsedFiles = append(parsedFiles, fileAST{
			relPath: relPath,
			pkgDir:  pkgDir,
			file:    fileNode,
			imports: importMap,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	// 2. First pass: Register all function and method declarations
	for _, fa := range parsedFiles {
		pkgName := fa.file.Name.Name
		isTestFile := strings.HasSuffix(fa.relPath, "_test.go")

		for _, decl := range fa.file.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok || funcDecl.Name == nil {
				continue
			}

			startPos := fset.Position(funcDecl.Pos())
			endPos := fset.Position(funcDecl.End())

			fnName := funcDecl.Name.Name
			var receiverStr string
			if funcDecl.Recv != nil && len(funcDecl.Recv.List) > 0 {
				receiverStr = exprToString(funcDecl.Recv.List[0].Type)
			}

			var funcID string
			if receiverStr != "" {
				funcID = pkgName + ".(" + receiverStr + ")." + fnName
			} else {
				funcID = pkgName + "." + fnName
			}

			isTest := isTestFile && strings.HasPrefix(fnName, "Test")

			info := &FuncInfo{
				ID:        funcID,
				Package:   pkgName,
				PkgDir:    fa.pkgDir,
				File:      fa.relPath,
				Name:      fnName,
				Receiver:  receiverStr,
				StartLine: startPos.Line,
				EndLine:   endPos.Line,
				IsTest:    isTest,
			}

			g.Functions[funcID] = info
			if isTest {
				g.AllTests = append(g.AllTests, funcID)
			}

			if receiverStr != "" {
				methodsByName[fnName] = append(methodsByName[fnName], info)
			}
		}
	}

	sort.Strings(g.AllTests)

	// 3. Second pass: Walk function bodies and extract call edges
	for _, fa := range parsedFiles {
		pkgName := fa.file.Name.Name

		for _, decl := range fa.file.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok || funcDecl.Body == nil {
				continue
			}

			var callerID string
			if funcDecl.Recv != nil && len(funcDecl.Recv.List) > 0 {
				recvStr := exprToString(funcDecl.Recv.List[0].Type)
				callerID = pkgName + ".(" + recvStr + ")." + funcDecl.Name.Name
			} else {
				callerID = pkgName + "." + funcDecl.Name.Name
			}

			calleeSet := make(map[string]bool)

			ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
				callExpr, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}

				switch fun := callExpr.Fun.(type) {
				case *ast.Ident:
					// Direct function call in same package or builtin
					if !builtinFuncs[fun.Name] {
						samePkgTarget := pkgName + "." + fun.Name
						if _, exists := g.Functions[samePkgTarget]; exists {
							calleeSet[samePkgTarget] = true
						}
					}

				case *ast.SelectorExpr:
					targetName := fun.Sel.Name
					if ident, ok := fun.X.(*ast.Ident); ok {
						// Subcase: imported package function call, e.g. math.Add
						if _, isImport := fa.imports[ident.Name]; isImport {
							importedTarget := ident.Name + "." + targetName
							if _, exists := g.Functions[importedTarget]; exists {
								calleeSet[importedTarget] = true
							}
						} else {
							// Subcase: method invocation on variable, e.g. c.Compute()
							if methods, hasMethods := methodsByName[targetName]; hasMethods {
								for _, m := range methods {
									calleeSet[m.ID] = true
								}
							}
						}
					} else {
						// Chained call, e.g. getCalc().Compute()
						if methods, hasMethods := methodsByName[targetName]; hasMethods {
							for _, m := range methods {
								calleeSet[m.ID] = true
							}
						}
					}
				}
				return true
			})

			for calleeID := range calleeSet {
				g.CallGraph[callerID] = append(g.CallGraph[callerID], calleeID)
				g.ReverseGraph[calleeID] = append(g.ReverseGraph[calleeID], callerID)
			}
		}
	}

	return g, nil
}

// MapChangedFunctions maps changed files and line ranges to the enclosing functions.
func (g *Graph) MapChangedFunctions(changedFiles []diffengine.ChangedFile) []string {
	var changed []string
	seen := make(map[string]bool)

	// Build map of file -> funcs for quick lookup
	funcsByFile := make(map[string][]*FuncInfo)
	for _, fn := range g.Functions {
		funcsByFile[fn.File] = append(funcsByFile[fn.File], fn)
	}

	for _, cf := range changedFiles {
		cleanPath := filepath.ToSlash(cf.Path)
		fileFuncs := funcsByFile[cleanPath]

		for _, fn := range fileFuncs {
			for _, lr := range cf.Lines {
				// Check for interval intersection
				if !(lr.End < fn.StartLine || lr.Start > fn.EndLine) {
					if !seen[fn.ID] {
						seen[fn.ID] = true
						changed = append(changed, fn.ID)
					}
					break
				}
			}
		}
	}

	sort.Strings(changed)
	return changed
}

// exprToString converts an ast.Expr representing a type into a canonical string.
func exprToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + exprToString(t.X)
	case *ast.SelectorExpr:
		return exprToString(t.X) + "." + t.Sel.Name
	default:
		return ""
	}
}

// DumpStats returns summary statistics about the call graph.
func (g *Graph) DumpStats() (int, int, int) {
	numFuncs := len(g.Functions)
	numEdges := 0
	for _, callees := range g.CallGraph {
		numEdges += len(callees)
	}
	return numFuncs, numEdges, len(g.AllTests)
}
