// Copyright 2011 Gary Burd
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

package main

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/gorilla/site/doc"
	godoc "go/doc"
	"net/http"
	"net/url"
	"path"
	"reflect"
	"regexp"
	"strings"
	"text/template"
	"time"
)

func mapFmt(kvs ...interface{}) (map[string]interface{}, error) {
	if len(kvs)%2 != 0 {
		return nil, errors.New("map requires even number of arguments.")
	}
	m := make(map[string]interface{})
	for i := 0; i < len(kvs); i += 2 {
		s, ok := kvs[i].(string)
		if !ok {
			return nil, errors.New("even args to map must be strings.")
		}
		m[s] = kvs[i+1]
	}
	return m, nil
}

// relativePathFmt formats an import path as HTML.
func relativePathFmt(importPath string, parentPath interface{}) string {
	if p, ok := parentPath.(string); ok && p != "" && strings.HasPrefix(importPath, p) {
		importPath = importPath[len(p)+1:]
	}
	return urlFmt(importPath)
}

// importPathFmt formats an import with zero width space characters to allow for breeaks.
func importPathFmt(importPath string) string {
	importPath = urlFmt(importPath)
	if len(importPath) > 45 {
		// Allow long import paths to break following "/"
		importPath = strings.Replace(importPath, "/", "/&#8203;", -1)
	}
	return importPath
}

// relativeTime formats the time t in nanoseconds as a human readable relative
// time.
func relativeTime(t time.Time) string {
	const day = 24 * time.Hour
	d := time.Now().Sub(t)
	switch {
	case d < time.Second:
		return "just now"
	case d < 2*time.Second:
		return "one second ago"
	case d < time.Minute:
		return fmt.Sprintf("%d seconds ago", d/time.Second)
	case d < 2*time.Minute:
		return "one minute ago"
	case d < time.Hour:
		return fmt.Sprintf("%d minutes ago", d/time.Minute)
	case d < 2*time.Hour:
		return "one hour ago"
	case d < day:
		return fmt.Sprintf("%d hours ago", d/time.Hour)
	case d < 2*day:
		return "one day ago"
	}
	return fmt.Sprintf("%d days ago", d/day)
}

var (
	h3Open     = []byte("<h3 ")
	h4Open     = []byte("<h2 ")
	h3Close    = []byte("</h3>")
	h4Close    = []byte("</h2>")
	rfcRE      = regexp.MustCompile(`RFC\s+(\d{3,4})`)
	rfcReplace = []byte(`<a href="http://tools.ietf.org/html/rfc$1">$0</a>`)
)

// commentFmt formats a source code control comment as HTML.
func commentFmt(v string) string {
	var buf bytes.Buffer
	godoc.ToHTML(&buf, v, nil)
	p := buf.Bytes()
	p = bytes.Replace(p, []byte("\t"), []byte("    "), -1)
	p = bytes.Replace(p, h3Open, h4Open, -1)
	p = bytes.Replace(p, h3Close, h4Close, -1)
	p = rfcRE.ReplaceAll(p, rfcReplace)
	return string(p)
}

// declFmt formats a Decl as HTML.
func declFmt(decl doc.Decl) string {
	var buf bytes.Buffer
	last := 0
	t := []byte(decl.Text)
	for _, a := range decl.Annotations {
		p := a.ImportPath
		if p != "" {
			p = "/" + p
		}
		template.HTMLEscape(&buf, t[last:a.Pos])
		//buf.WriteString(`<a href="`)
		//buf.WriteString(urlFmt(p))
		//buf.WriteByte('#')
		//buf.WriteString(urlFmt(a.Name))
		//buf.WriteString(`">`)
		template.HTMLEscape(&buf, t[a.Pos:a.End])
		//buf.WriteString(`</a>`)
		last = a.End
	}
	template.HTMLEscape(&buf, t[last:])
	return buf.String()
}

func commandNameFmt(pdoc *doc.Package) string {
	_, name := path.Split(pdoc.ImportPath)
	return template.HTMLEscapeString(name)
}

func breadcrumbsFmt(pdoc *doc.Package) string {
	importPath := []byte(pdoc.ImportPath)
	var buf bytes.Buffer
	i := 0
	j := len(pdoc.ProjectRoot)
	switch {
	case j == 0:
		buf.WriteString("<a href=\"/-/go\" title=\"Standard Packages\">☆</a> ")
		j = bytes.IndexByte(importPath, '/')
	case j >= len(importPath):
		j = -1
	}
	for j > 0 {
		buf.WriteString(`<a href="/`)
		buf.WriteString(urlFmt(string(importPath[:i+j])))
		buf.WriteString(`">`)
		template.HTMLEscape(&buf, importPath[i:i+j])
		buf.WriteString(`</a>/`)
		i = i + j + 1
		j = bytes.IndexByte(importPath[i:], '/')
	}
	template.HTMLEscape(&buf, importPath[i:])
	return buf.String()
}

func urlFmt(path string) string {
	u := url.URL{Path: path}
	return u.String()
}

func executeTemplate(w http.ResponseWriter, name string, status int, data interface{}) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	return tpls[name].ExecuteTemplate(w, "base", data)
}

// eq reports whether the first argument is equal to
// any of the remaining arguments.
// https://groups.google.com/group/golang-nuts/msg/a720bf35f454288b
func eq(args ...interface{}) bool {
	if len(args) == 0 {
		return false
	}
	x := args[0]
	switch x := x.(type) {
	case string, int, int64, byte, float32, float64:
		for _, y := range args[1:] {
			if x == y {
				return true
			}
		}
		return false
	}
	for _, y := range args[1:] {
		if reflect.DeepEqual(x, y) {
			return true
		}
	}
	return false
}

var tpls = map[string]*template.Template{
	"404":      newTemplate("templates/base.html", "templates/404.html"),
	"home":     newTemplate("templates/base.html", "templates/home.html"),
	"pkg":      newTemplate("templates/base.html", "templates/pkg.html"),
	"pkgIndex": newTemplate("templates/base.html", "templates/pkgIndex.html"),
}

var funcs = template.FuncMap{
	"comment":      commentFmt,
	"decl":         declFmt,
	"eq":           eq,
	"map":          mapFmt,
	"breadcrumbs":  breadcrumbsFmt,
	"commandName":  commandNameFmt,
	"relativePath": relativePathFmt,
	"relativeTime": relativeTime,
	"importPath":   importPathFmt,
	"url":          urlFmt,
}

func newTemplate(files ...string) *template.Template {
	return template.Must(template.New("*").Funcs(funcs).ParseFiles(files...))
}
