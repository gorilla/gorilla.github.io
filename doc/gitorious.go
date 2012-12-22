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

package doc

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"io/ioutil"
	"net/http"
	"path"
	"regexp"
	"strings"
)

var gitoriousPattern = regexp.MustCompile(`^git\.gitorious\.org/([a-z0-9A-Z_.\-]+)/([a-z0-9A-Z_.\-]+)\.git(/[a-z0-9A-Z_.\-/]*)?$`)

func getGitoriousDoc(client *http.Client, m []string, savedEtag string) (*Package, error) {

	importPath := m[0]
	projectRoot := "git.gitorious.org/" + m[1] + "/" + m[2] + ".git"
	projectName := m[2]
	projectURL := "https://gitorious.org/" + m[1] + "/" + m[2] + "/"
	dir := normalizeDir(m[3])

	p, etag, err := httpGetBytesCompare(client, "https://gitorious.org/"+m[1]+"/"+m[2]+"/archive-tarball/master", savedEtag)
	if err != nil {
		return nil, err
	}

	gzr, err := gzip.NewReader(bytes.NewReader(p))
	if err != nil {
		return nil, err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	inTree := false
	prefix := m[1] + "-" + m[2] + "/" + dir
	var files []*source
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(hdr.Name, prefix) {
			continue
		}
		name := hdr.Name[len(prefix):]
		if !isDocFile(name) {
			continue
		}
		inTree = true
		if d, _ := path.Split(name); d == "" {
			b, err := ioutil.ReadAll(tr)
			if err != nil {
				return nil, err
			}
			files = append(files, &source{
				name:      name,
				browseURL: "https://gitorious.org/" + m[1] + "/" + m[2] + "/blobs/master/" + dir + name,
				data:      b})
		}
	}

	if !inTree {
		return nil, ErrPackageNotFound
	}

	return buildDoc(importPath, projectRoot, projectName, projectURL, etag, "#line%d", files)
}
