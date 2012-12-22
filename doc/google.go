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

package doc

import (
	"net/http"
	"regexp"
	"strings"
)

var googleRepoRe = regexp.MustCompile(`id="checkoutcmd">(hg|git|svn)`)
var googleFilePattern = regexp.MustCompile(`<li><a href="([^"/]+)"`)
var googlePattern = regexp.MustCompile(`^code\.google\.com/p/([a-z0-9\-]+)(\.[a-z0-9\-]+)?(/[a-z0-9A-Z_.\-/]+)?$`)

func getGoogleDoc(client *http.Client, m []string, savedEtag string) (*Package, error) {

	importPath := m[0]
	projectRoot := "code.google.com/p/" + m[1] + m[2]
	projectName := m[1] + m[2]
	projectURL := "https://code.google.com/p/" + m[1] + "/"

	repo := m[1]
	subrepo := m[2]
	if len(subrepo) > 0 {
		subrepo = subrepo[1:] + "."
	}

	dir := normalizeDir(m[3])

	// Scrape the HTML project page to find the VCS.
	p, err := httpGetBytes(client, "http://code.google.com/p/"+repo+"/source/checkout")
	if err != nil {
		return nil, err
	}

	var vcs string
	if m := googleRepoRe.FindSubmatch(p); m != nil {
		vcs = string(m[1])
	} else {
		return nil, ErrPackageNotFound
	}

	// Scrape the repo browser to find individual Go files.
	p, etag, err := httpGetBytesCompare(client, "http://"+subrepo+repo+".googlecode.com/"+vcs+"/"+dir, savedEtag)
	if err != nil {
		return nil, err
	}

	var files []*source
	query := ""
	if subrepo != "" {
		query = "?repo=" + subrepo[:len(subrepo)-1]
	}
	for _, m := range googleFilePattern.FindAllSubmatch(p, -1) {
		fname := string(m[1])
		if isDocFile(fname) {
			files = append(files, &source{
				name:      fname,
				browseURL: "http://code.google.com/p/" + repo + "/source/browse/" + dir + fname + query,
				rawURL:    "http://" + subrepo + repo + ".googlecode.com/" + vcs + "/" + dir + fname,
			})
		}
	}

	if err := fetchFiles(client, files, nil); err != nil {
		return nil, err
	}

	return buildDoc(importPath, projectRoot, projectName, projectURL, etag, "#%d", files)
}

func getStandardDoc(client *http.Client, importPath string, savedEtag string) (*Package, error) {

	// Scrape the repo browser to find individual Go files.
	p, etag, err := httpGetBytesCompare(client, "http://go.googlecode.com/hg-history/release/src/pkg/"+importPath+"/", savedEtag)
	if err != nil {
		return nil, err
	}

	var files []*source
	for _, m := range googleFilePattern.FindAllSubmatch(p, -1) {
		fname := strings.Split(string(m[1]), "?")[0]
		if isDocFile(fname) {
			files = append(files, &source{
				name:      fname,
				browseURL: "http://code.google.com/p/go/source/browse/src/pkg/" + importPath + "/" + fname + "?name=release",
				rawURL:    "http://go.googlecode.com/hg-history/release/src/pkg/" + importPath + "/" + fname,
			})
		}
	}

	if err := fetchFiles(client, files, nil); err != nil {
		return nil, err
	}

	return buildDoc(importPath, "", "Go", "https://code.google.com/p/go", etag, "#%d", files)
}
