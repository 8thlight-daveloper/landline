//go:build ignore

// Reads Go file paths from standard input, one per line, and prints path:line:name for each use of a function of the
// time package that reads the clock. A dot import of time prints a ".", since its calls cannot be told from others.
package main

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
)

var clock = map[string]bool{
	"Now": true, "Since": true, "Until": true, "Sleep": true, "After": true, "Tick": true,
	"NewTimer": true, "NewTicker": true, "AfterFunc": true,
}

func main() {
	fset := token.NewFileSet()
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		path := scanner.Text()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		local := ""
		for _, spec := range file.Imports {
			if spec.Path.Value != `"time"` {
				continue
			}
			local = "time"
			if spec.Name != nil {
				local = spec.Name.Name
			}
			if local == "." {
				fmt.Printf("%s:%d:.\n", path, fset.Position(spec.Pos()).Line)
			}
		}
		if local == "" || local == "_" || local == "." {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			name, ok := selector.X.(*ast.Ident)
			if ok && name.Name == local && name.Obj == nil && clock[selector.Sel.Name] {
				fmt.Printf("%s:%d:%s\n", path, fset.Position(selector.Pos()).Line, selector.Sel.Name)
			}
			return true
		})
	}
}
