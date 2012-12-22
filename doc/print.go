// Copyright 2012 Gary Burd
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

// +build ignore

// Command print fetches and prints package documentation.
//
// Usage: go run print.go importPath
package main

import (
	"fmt"
	"github.com/garyburd/gopkgdoc/doc"
	"log"
	"net/http"
	"os"
	"strings"
)

func indent(s string, n int) string {
	const space = "                       "
	return strings.Replace(strings.TrimSpace(s), "\n", "\n"+space[:n], -1)
}

func main() {
	if len(os.Args) != 2 {
		log.Fatal("Usage: go run print.go importPath")
	}
	dpkg, err := doc.Get(http.DefaultClient, os.Args[1], "")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("ImportPath:  ", dpkg.ImportPath)
	fmt.Println("ProjectRoot: ", dpkg.ProjectRoot)
	fmt.Println("ProjectName: ", dpkg.ProjectName)
	fmt.Println("ProjectURL:  ", dpkg.ProjectURL)
	fmt.Println("Updated:     ", dpkg.Updated)
	fmt.Println("Etag:        ", dpkg.Etag)
	fmt.Println("Name:        ", dpkg.Name)
	fmt.Println("IsCmd:       ", dpkg.IsCmd)
	fmt.Println("Synopsis:    ", dpkg.Synopsis)
	fmt.Println("Doc:         ", indent(dpkg.Doc, 14))
	fmt.Println("Errors:")
	for _, s := range dpkg.Errors {
		fmt.Println("    ", s)
	}
	fmt.Println("Files:")
	for _, f := range dpkg.Files {
		fmt.Println("    ", f)
	}
	fmt.Println("Imports:")
	for _, i := range dpkg.Imports {
		fmt.Println("    ", i)
	}
	fmt.Println("TestImports:")
	for _, i := range dpkg.TestImports {
		fmt.Println("    ", i)
	}
	for _, c := range dpkg.Consts {
		fmt.Println("Const:")
		fmt.Println("    Decl:  ", indent(c.Decl.Text, 12))
		fmt.Println("    Doc:   ", indent(c.Doc, 12))
		fmt.Println("    URL:   ", c.URL)
	}
	for _, c := range dpkg.Vars {
		fmt.Println("Var:")
		fmt.Println("    Decl:  ", indent(c.Decl.Text, 12))
		fmt.Println("    Doc:   ", indent(c.Doc, 12))
		fmt.Println("    URL:   ", c.URL)
	}
	for _, f := range dpkg.Funcs {
		fmt.Println("Func:")
		fmt.Println("    Decl:  ", indent(f.Decl.Text, 12))
		fmt.Println("    Doc:   ", indent(f.Doc, 12))
		fmt.Println("    URL:   ", f.URL)
	}
	for _, t := range dpkg.Types {
		fmt.Println("Type:")
		fmt.Println("    Decl:  ", indent(t.Decl.Text, 12))
		fmt.Println("    Doc:   ", indent(t.Doc, 12))
		fmt.Println("    URL:   ", t.URL)
		for _, f := range t.Funcs {
			fmt.Println("    Func:")
			fmt.Println("        Decl:  ", indent(f.Decl.Text, 16))
			fmt.Println("        Doc:   ", indent(f.Doc, 16))
			fmt.Println("        URL:   ", f.URL)
		}
		for _, m := range t.Methods {
			fmt.Println("    Method:")
			fmt.Println("        Decl:  ", indent(m.Decl.Text, 16))
			fmt.Println("        Doc:   ", indent(m.Doc, 16))
			fmt.Println("        URL:   ", m.URL)
		}
	}
}
