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
	"encoding/json"
	"net/http"
	"path"
	"regexp"
	"strings"
)

var githubRawHeader = http.Header{"Accept": {"application/vnd.github-blob.raw"}}
var githubPattern = regexp.MustCompile(`^github\.com/([a-z0-9A-Z_.\-]+)/([a-z0-9A-Z_.\-]+)(/[a-z0-9A-Z_.\-/]*)?$`)

func getGithubDoc(client *http.Client, m []string, savedEtag string) (*Package, error) {
	importPath := m[0]
	projectRoot := "github.com/" + m[1] + "/" + m[2]
	projectName := m[2]
	projectURL := "https://github.com/" + m[1] + "/" + m[2] + "/"
	userRepo := m[1] + "/" + m[2]
	dir := normalizeDir(m[3])

	p, err := httpGetBytes(client, "https://api.github.com/repos/"+userRepo+"/git/refs")
	if err != nil {
		return nil, err
	}

	var refs []*struct {
		Object struct {
			Type string
			Sha  string
			Url  string
		}
		Ref string
		Url string
	}

	if err := json.Unmarshal(p, &refs); err != nil {
		return nil, err
	}

	etag := ""
	treeName := "master"
	for _, ref := range refs {
		if ref.Ref == "refs/heads/go1" || ref.Ref == "refs/tags/go1" {
			treeName = "go1"
			etag = ref.Object.Sha + ref.Ref[len("refs"):]
			break
		} else if ref.Ref == "refs/heads/master" {
			etag = ref.Object.Sha
		}
	}

	if etag == savedEtag {
		return nil, ErrPackageNotModified
	}

	p, err = httpGetBytes(client, "https://api.github.com/repos/"+userRepo+"/git/trees/"+treeName+"?recursive=1")
	if err != nil {
		return nil, err
	}

	var tree struct {
		Tree []struct {
			Url  string
			Path string
			Type string
		}
	}
	if err := json.Unmarshal(p, &tree); err != nil {
		return nil, err
	}

	inTree := false
	var files []*source
	for _, node := range tree.Tree {
		if node.Type != "blob" ||
			!isDocFile(node.Path) ||
			!strings.HasPrefix(node.Path, dir) {
			continue
		}
		inTree = true
		if d, f := path.Split(node.Path); d == dir {
			files = append(files, &source{
				name:      f,
				browseURL: "https://github.com/" + userRepo + "/blob/" + treeName + "/" + node.Path,
				rawURL:    node.Url,
			})
		}
	}

	if !inTree {
		return nil, ErrPackageNotFound
	}

	if err := fetchFiles(client, files, githubRawHeader); err != nil {
		return nil, err
	}

	return buildDoc(importPath, projectRoot, projectName, projectURL, etag, "#L%d", files)
}
