package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

var talosVersion = flag.String("talos-version", "main", "the desired Talos version (branch or tag)")

func changePackageName(node *ast.File, newPackageName string) {
	node.Name = ast.NewIdent(newPackageName)
}

func addFieldToStruct(node *ast.File, varName, fieldType, fieldName string) {
	// Variable to track if the variable or type is found
	var found bool

	// Inspect nodes to find and modify the target struct declaration or its type
	ast.Inspect(node, func(n ast.Node) bool {
		switch decl := n.(type) {
		case *ast.GenDecl:
			if decl.Tok == token.TYPE {
				for _, spec := range decl.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok || !strings.HasSuffix(ts.Name.Name, "Type") {
						continue
					}
					if typeName := strings.TrimSuffix(ts.Name.Name, "Type"); typeName != varName {
						continue
					}

					st, ok := ts.Type.(*ast.StructType)
					if !ok {
						continue
					}

					// Add field to the found struct type
					field := &ast.Field{
						Names: []*ast.Ident{ast.NewIdent(fieldName)},
						Type:  ast.NewIdent(fieldType),
					}
					st.Fields.List = append(st.Fields.List, field)
					found = true
					return false // stop searching, type found and updated
				}
			} else if decl.Tok == token.VAR {
				for _, spec := range decl.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok || len(vs.Names) != 1 || vs.Names[0].Name != varName {
						continue
					}

					st, ok := vs.Type.(*ast.StructType)
					if !ok {
						continue
					}

					// Add field to the found struct variable
					field := &ast.Field{
						Names: []*ast.Ident{ast.NewIdent(fieldName)},
						Type:  ast.NewIdent(fieldType),
					}
					st.Fields.List = append(st.Fields.List, field)
					found = true
					return false // stop searching, variable found and updated
				}
			}
		}
		return true
	})

	// If the struct or type is not found, create a new variable struct
	if !found {
		newField := &ast.Field{
			Names: []*ast.Ident{ast.NewIdent(fieldName)},
			Type:  ast.NewIdent(fieldType),
		}
		newStruct := &ast.StructType{
			Fields: &ast.FieldList{
				List: []*ast.Field{newField},
			},
		}
		newSpec := &ast.ValueSpec{
			Names: []*ast.Ident{ast.NewIdent(varName)},
			Type:  newStruct,
		}
		newDecl := &ast.GenDecl{
			Tok:   token.VAR,
			Specs: []ast.Spec{newSpec},
		}

		node.Decls = append(node.Decls, newDecl)
		fmt.Println("New struct variable created:", varName)
	}
}

func prependStmtToInit(node *ast.File, cmdName string) {
	ast.Inspect(node, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if ok && fn.Name.Name == "init" {
			stmt := &ast.ExprStmt{
				X: &ast.CallExpr{
					Fun: &ast.SelectorExpr{
						X: &ast.SelectorExpr{
							X:   ast.NewIdent(cmdName + "Cmd"),
							Sel: ast.NewIdent("Flags()"),
						},
						Sel: ast.NewIdent("StringSliceVarP"),
					},
					Args: []ast.Expr{
						&ast.UnaryExpr{
							Op: token.AND,
							X:  ast.NewIdent(cmdName + "CmdFlags.configFiles"),
						},
						ast.NewIdent(`"file"`),
						ast.NewIdent(`"f"`),
						ast.NewIdent("nil"),
						ast.NewIdent(`"specify config files or patches in a YAML file (can specify multiple)"`),
					},
				},
			}
			fn.Body.List = append([]ast.Stmt{stmt}, fn.Body.List...)
			return false
		}
		return true
	})
}

func insertInitCode(node *ast.File, cmdName, initCode string) {
	anonFuncCode := fmt.Sprintf(`func() { %s }`, initCode)

	initCodeExpr, err := parser.ParseExpr(anonFuncCode)
	if err != nil {
		log.Fatalf("Failed to parse init code: %v", err)
	}

	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			if x.Name.Name == "init" {
				if x.Body != nil {
					initFunc, ok := initCodeExpr.(*ast.FuncLit)
					if !ok {
						log.Fatalf("Failed to extract function body from init code expression")
					}

					x.Body.List = append(initFunc.Body.List, x.Body.List...)
				}
			}
		}
		return true
	})
}

func processFile(filename, cmdName string) {
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Fatalf("Failed to read the file: %v", err)
	}
	src := string(content)

	src = strings.ReplaceAll(src, "\"f\"", "\"F\"")
	src = strings.ReplaceAll(src, "github.com/siderolabs/talos/internal", "github.com/aenix-io/talm/internal")

	// Create a new set of tokens and parse the source code
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		log.Fatalf("Failed to parse file: %v", err)
	}

	changePackageName(node, "commands")
	addFieldToStruct(node, cmdName+"CmdFlags", "[]string", "configFiles")

	initCode := fmt.Sprintf(`%sCmd.Flags().StringSliceVarP(&%sCmdFlags.configFiles, "file", "f", nil, "specify config files or patches in a YAML file (can specify multiple)")
	%sCmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		nodesFromArgs := len(GlobalArgs.Nodes) > 0
		endpointsFromArgs := len(GlobalArgs.Endpoints) > 0
		for _, configFile := range %sCmdFlags.configFiles {
			if err :=	processModelineAndUpdateGlobals(configFile,	nodesFromArgs, endpointsFromArgs, false); err != nil {
				return err
			}
		}
		return nil
	}
	`, cmdName, cmdName, cmdName, cmdName)

	if cmdName == "etcd" {
		for _, subCmdName := range []string{"etcdAlarmCmd", "etcdDefragCmd", "etcdForfeitLeadershipCmd", "etcdLeaveCmd", "etcdMemberListCmd", "etcdMemberRemoveCmd", "etcdSnapshotCmd", "etcdStatusCmd"} {
			initCode = fmt.Sprintf("%s\n%s", initCode, fmt.Sprintf(`
	%s.Flags().StringSliceVarP(&etcdCmdFlags.configFiles, "file", "f", nil, "specify config files or patches in a YAML file (can specify multiple)")
	%s.PreRunE = etcdCmd.PreRunE
		`, subCmdName, subCmdName))
		}
	}
	if cmdName == "image" {
		for _, subCmdName := range []string{"imageDefaultCmd", "imageListCmd", "imagePullCmd"} {
			initCode = fmt.Sprintf("%s\n%s", initCode, fmt.Sprintf(`
	%s.Flags().StringSliceVarP(&etcdCmdFlags.configFiles, "file", "f", nil, "specify config files or patches in a YAML file (can specify multiple)")
	%s.PreRunE = etcdCmd.PreRunE
		`, subCmdName, subCmdName))
		}
	}

	if cmdName == "etcd" {
		for _, subCmdName := range []string{"etcdAlarmListCmd", "etcdAlarmDisarmCmd"} {
			initCode = fmt.Sprintf("%s\n%s", initCode, fmt.Sprintf(`
	%s.Flags().StringSliceVarP(&etcdCmdFlags.configFiles, "file", "f", nil, "specify config files or patches in a YAML file (can specify multiple)")
	%s.PreRunE = etcdCmd.PreRunE
		`, subCmdName, subCmdName))
		}
	}

	insertInitCode(node, cmdName, initCode)

	var buf bytes.Buffer
	comment := fmt.Sprintf("// Code generated by go run tools/import_commands.go --talos-version %s %s\n// DO NOT EDIT.\n\n", *talosVersion, cmdName)
	buf.WriteString(comment)

	if err := format.Node(&buf, fset, node); err != nil {
		log.Fatalf("Failed to format the AST: %v", err)
	}

	if err := ioutil.WriteFile(filename, buf.Bytes(), 0644); err != nil {
		log.Fatalf("Failed to write the modified file: %v", err)
	}

	log.Printf("File %s updated successfully.", filename)
}

func main() {
	flag.Parse()
	url := fmt.Sprintf("https://github.com/siderolabs/talos/raw/%s/cmd/talosctl/cmd/talos/", *talosVersion)

	args := flag.Args()
	if len(args) == 0 {
		fmt.Println("Please provide commands to import")
		return
	}

	for _, cmd := range args {
		srcName := cmd + ".go"
		dstName := "pkg/commands/imported_" + srcName

		err := downloadFile(srcName, dstName, url)
		if err != nil {
			log.Fatalf("Error downloading file: %v", err)
		}

		log.Printf("File %s succefully downloaded to %s", srcName, dstName)

		cmdName := strings.TrimSuffix(filepath.Base(srcName), ".go")
		if cmdName == "list" {
			cmdName = "ls"
		}
		processFile(dstName, cmdName)
	}
}

func downloadFile(srcName, dstName string, url string) error {
	resp, err := http.Get(url + "/" + srcName)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	file, err := os.Create(dstName)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return err
	}

	return nil
}
