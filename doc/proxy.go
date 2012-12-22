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
	"compress/gzip"
	"io"
	"io/ioutil"
	"net/http"
)

func getProxyDoc(client *http.Client, importPath, projectRoot, projectName, projectURL, etag string) (*Package, error) {

	rc, err := httpGet(client, "http://go-get.danga.com/"+importPath)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	gzr, err := gzip.NewReader(rc)
	if err != nil {
		return nil, err
	}
	defer gzr.Close()
	tr := tar.NewReader(gzr)
	var files []*source
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if !isDocFile(hdr.Name) {
			continue
		}
		b, err := ioutil.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		files = append(files, &source{
			name:      hdr.Name,
			browseURL: "http://gosourcefile.appspot.com/" + importPath + "/" + hdr.Name,
			data:      b})
	}
	return buildDoc(importPath, projectRoot, projectName, projectURL, etag, "#L%d", files)
}
