// (c) Copyright 2016 Hewlett Packard Enterprise Development LP
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"sort"
	"strings"
)

type command func(args ...string)
type utilities struct {
	commands map[string]command
	call     []string
}

// Custom commands / utilities to run instead of default analyzer
func newUtils() *utilities {
	utils := make(map[string]command)
	utils["ast"] = dumpAst
	utils["callobj"] = dumpCallObj
	utils["uses"] = dumpUses
	utils["types"] = dumpTypes
	utils["defs"] = dumpDefs
	utils["comments"] = dumpComments
	utils["imports"] = dumpImports
	return &utilities{utils, make([]string, 0)}
}

func (u *utilities) String() string {
	keys := make([]string, 0, len(u.commands))
	for k := range u.commands {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return strings.Join(keys, ", ")
}

func (u *utilities) Set(opt string) error {
	if _, ok := u.commands[opt]; !ok {
		return fmt.Errorf("valid tools are: %s", u.String())

	}
	u.call = append(u.call, opt)
	return nil
}

func (u *utilities) run(args ...string) {
	for _, util := range u.call {
		if cmd, ok := u.commands[util]; ok {
			cmd(args...)
		}
	}
}

func shouldSkip(path string) bool {
	st, e := os.Stat(path)
	if e != nil {
		// #nosec
		fmt.Fprintf(os.Stderr, "Skipping: %s - %s\n", path, e)
		return true
	}
	if st.IsDir() {
		// #nosec
		fmt.Fprintf(os.Stderr, "Skipping: %s - directory\n", path)
		return true
	}
	return false
}

func dumpAst(files ...string) {
	for _, arg := range files {
		// Ensure file exists and not a directory
		if shouldSkip(arg) {
			continue
		}

		// Create the AST by parsing src.
		fset := token.NewFileSet() // positions are relative to fset
		f, err := parser.ParseFile(fset, arg, nil, 0)
		if err != nil {
			// #nosec
			fmt.Fprintf(os.Stderr, "Unable to parse file %s\n", err)
			continue
		}

		// Print the AST. #nosec
		ast.Print(fset, f)
	}
}

type context struct {
	fileset  *token.FileSet
	comments ast.CommentMap
	info     *types.Info
	pkg      *types.Package
	config   *types.Config
	root     *ast.File
}

func createContext(filename string) *context {
	fileset := token.NewFileSet()
	root, e := parser.ParseFile(fileset, filename, nil, parser.ParseComments)
	if e != nil {
		// #nosec
		fmt.Fprintf(os.Stderr, "Unable to parse file: %s. Reason: %s\n", filename, e)
		return nil
	}
	comments := ast.NewCommentMap(fileset, root, root.Comments)
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
		Scopes:     make(map[ast.Node]*types.Scope),
		Implicits:  make(map[ast.Node]types.Object),
	}
	config := types.Config{Importer: importer.Default()}
	pkg, e := config.Check("main.go", fileset, []*ast.File{root}, info)
	if e != nil {
		// #nosec
		fmt.Fprintf(os.Stderr, "Type check failed for file: %s. Reason: %s\n", filename, e)
		return nil
	}
	return &context{fileset, comments, info, pkg, &config, root}
}

func printObject(obj types.Object) {
	fmt.Println("OBJECT")
	if obj == nil {
		fmt.Println("object is nil")
		return
	}
	fmt.Printf("   Package = %v\n", obj.Pkg())
	if obj.Pkg() != nil {
		fmt.Println("   Path = ", obj.Pkg().Path())
		fmt.Println("   Name = ", obj.Pkg().Name())
		fmt.Println("   String = ", obj.Pkg().String())
	}
	fmt.Printf("   Name = %v\n", obj.Name())
	fmt.Printf("   Type = %v\n", obj.Type())
	fmt.Printf("   Id = %v\n", obj.Id())
}

func checkContext(ctx *context, file string) bool {
	// #nosec
	if ctx == nil {
		fmt.Fprintln(os.Stderr, "Failed to create context for file: ", file)
		return false
	}
	return true
}

func dumpCallObj(files ...string) {

	for _, file := range files {
		if shouldSkip(file) {
			continue
		}
		context := createContext(file)
		if !checkContext(context, file) {
			return
		}
		ast.Inspect(context.root, func(n ast.Node) bool {
			var obj types.Object
			switch node := n.(type) {
			case *ast.Ident:
				obj = context.info.ObjectOf(node) //context.info.Uses[node]
			case *ast.SelectorExpr:
				obj = context.info.ObjectOf(node.Sel) //context.info.Uses[node.Sel]
			default:
				obj = nil
			}
			if obj != nil {
				printObject(obj)
			}
			return true
		})
	}
}

func dumpUses(files ...string) {
	for _, file := range files {
		if shouldSkip(file) {
			continue
		}
		context := createContext(file)
		if !checkContext(context, file) {
			return
		}
		idents := make([]*ast.Ident, 0, len(context.info.Uses))
		for ident := range context.info.Uses {
			idents = append(idents, ident)
		}
		sort.Slice(idents, func(i, j int) bool { return idents[i].String() < idents[j].String() })
		for _, ident := range idents {
			fmt.Printf("IDENT: %v, OBJECT: %v\n", ident, context.info.Uses[ident])
		}
	}
}

func dumpTypes(files ...string) {
	for _, file := range files {
		if shouldSkip(file) {
			continue
		}
		context := createContext(file)
		if !checkContext(context, file) {
			return
		}
		exprs := make([]ast.Expr, 0, len(context.info.Types))
		for expr := range context.info.Types {
			exprs = append(exprs, expr)
		}
		sort.Slice(exprs, func(i, j int) bool { return fmt.Sprintf("%v", exprs[i]) < fmt.Sprintf("%v", exprs[j]) })
		for _, expr := range exprs {
			fmt.Printf("EXPR: %v, TYPE: %v\n", expr, context.info.Types[expr])
		}
	}
}

func dumpDefs(files ...string) {
	for _, file := range files {
		if shouldSkip(file) {
			continue
		}
		context := createContext(file)
		if !checkContext(context, file) {
			return
		}
		idents := make([]*ast.Ident, 0, len(context.info.Defs))
		for ident := range context.info.Defs {
			idents = append(idents, ident)
		}
		sort.Slice(idents, func(i, j int) bool { return idents[i].String() < idents[j].String() })
		for _, ident := range idents {
			fmt.Printf("IDENT: %v, OBJECT: %v\n", ident, context.info.Defs[ident])
		}
	}
}

func dumpComments(files ...string) {
	for _, file := range files {
		if shouldSkip(file) {
			continue
		}
		context := createContext(file)
		if !checkContext(context, file) {
			return
		}
		for _, group := range context.comments.Comments() {
			fmt.Println(group.Text())
		}
	}
}

func dumpImports(files ...string) {
	for _, file := range files {
		if shouldSkip(file) {
			continue
		}
		context := createContext(file)
		if !checkContext(context, file) {
			return
		}
		for _, pkg := range context.pkg.Imports() {
			fmt.Println(pkg.Path(), pkg.Name())
			for _, name := range pkg.Scope().Names() {
				fmt.Println("  => ", name)
			}
		}
	}
}

func main() {
	tools := newUtils()
	flag.Var(tools, "tool", "Utils to assist with rule development")
	flag.Parse()

	if len(tools.call) > 0 {
		tools.run(flag.Args()...)
		os.Exit(0)
	}
}
