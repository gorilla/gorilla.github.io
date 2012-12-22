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

// +build appengine

package app

import (
	"bytes"
	"doc"
	"encoding/gob"
	"path"
	"strings"
	"time"

	"appengine"
	"appengine/datastore"
	"appengine/memcache"
)

const (
	packageListKey       = "pkglist2"
	projectListKeyPrefix = "proj2:"
)

type Package struct {
	ImportPath  string `datastore:"-"`
	Synopsis    string `datastore:",noindex"`
	PackageName string `datastore:",noindex"`
	IsCmd       bool   `datastore:",noindex"`
	Hide        bool
	IndexTokens []string
}

func (p *Package) ShortPath() string {
	return p.ImportPath[len("github.com/gorilla/"):]
}

type Doc struct {
	Version string `datastore:",noindex"`
	Gob     []byte `datastore:",noindex"`
}

func loadDoc(c appengine.Context, importPath string) (*doc.Package, string, error) {
	var d Doc
	err := datastore.Get(c, datastore.NewKey(c, "Doc", importPath, 0, nil), &d)
	if err == datastore.ErrNoSuchEntity {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	if d.Version != doc.PackageVersion {
		return nil, "", nil
	}
	var p doc.Package
	err = gob.NewDecoder(bytes.NewBuffer(d.Gob)).Decode(&p)
	return &p, p.Etag, err
}

func removeDoc(c appengine.Context, importPath string) {
	err := datastore.Delete(c, datastore.NewKey(c, "Doc", importPath, 0, nil))
	if err != nil && err != datastore.ErrNoSuchEntity {
		c.Errorf("Delete(%s) -> %v", importPath, err)
	}
}

func queryPackages(c appengine.Context, cacheKey string, query *datastore.Query) ([]*Package, error) {
	var pkgs []*Package
	item, err := cacheGet(c, cacheKey, &pkgs)
	if err == memcache.ErrCacheMiss {
		keys, err := query.GetAll(c, &pkgs)
		if err != nil {
			return nil, err
		}
		for i := range keys {
			importPath := keys[i].StringID()
			if importPath[0] == '/' {
				// Standard packages start with "/"
				importPath = importPath[1:]
			}
			pkgs[i].ImportPath = importPath
		}
		item.Expiration = time.Hour
		item.Object = pkgs
		if err := cacheSafeSet(c, item); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	return pkgs, nil
}

func (pkg *Package) equal(other *Package) bool {
	if pkg.Synopsis != other.Synopsis {
		return false
	}
	if pkg.Hide != other.Hide {
		return false
	}
	if pkg.IsCmd != other.IsCmd {
		return false
	}
	if len(pkg.IndexTokens) != len(other.IndexTokens) {
		return false
	}
	for i := range pkg.IndexTokens {
		if pkg.IndexTokens[i] != other.IndexTokens[i] {
			return false
		}
	}
	return true
}

// updatePackage updates the package in the datastore and clears memcache as
// needed.
func updatePackage(c appengine.Context, importPath string, pdoc *doc.Package) error {

	var pkg *Package
	if pdoc != nil && pdoc.Name != "" {

		indexTokens := make([]string, 0, 3)
		if pdoc.ProjectRoot != "" {
			indexTokens = append(indexTokens, strings.ToLower(pdoc.ProjectRoot))
		}

		hide := false
		switch {
		case strings.HasPrefix(importPath, "code.google.com/p/go/"):
			hide = true
		case pdoc.ProjectRoot == "":
			// standard packages
			hide = true
			indexTokens = append(indexTokens, strings.ToLower(pdoc.Name))
		case pdoc.IsCmd:
			// Hide if command does not have a synopsis or doc with more than one sentence.
			i := strings.Index(pdoc.Doc, ".")
			hide = pdoc.Synopsis == "" || i < 0 || i == len(pdoc.Doc)-1
			if !hide {
				_, name := path.Split(strings.ToLower(pdoc.ImportPath))
				indexTokens = append(indexTokens, name)
			}
		default:
			// Hide if no exports.
			hide = len(pdoc.Consts) == 0 && len(pdoc.Funcs) == 0 && len(pdoc.Types) == 0 && len(pdoc.Vars) == 0
			if !hide {
				_, name := path.Split(strings.ToLower(pdoc.ImportPath))
				indexTokens = append(indexTokens, name)
				name = strings.ToLower(pdoc.Name)
				if name != indexTokens[len(indexTokens)-1] {
					indexTokens = append(indexTokens, name)
				}
			}
		}

		pkg = &Package{
			Synopsis:    pdoc.Synopsis,
			PackageName: pdoc.Name,
			IsCmd:       pdoc.IsCmd,
			Hide:        hide,
			IndexTokens: indexTokens,
		}
	}

	// Update doc blob.

	key := datastore.NewKey(c, "Doc", importPath, 0, nil)
	if pkg == nil {
		if err := datastore.Delete(c, key); err != datastore.ErrNoSuchEntity && err != nil {
			c.Errorf("Delete(%s) -> %v", importPath, err)
		}
	} else {
		var buf bytes.Buffer
		err := gob.NewEncoder(&buf).Encode(pdoc)
		if err != nil {
			return err
		}

		if buf.Len() > 800000 {
			pdoc.Errors = append(pdoc.Errors, "Documentation truncated.")
			pdoc.Vars = nil
			pdoc.Funcs = nil
			pdoc.Types = nil
			pdoc.Consts = nil
			buf.Reset()
			err := gob.NewEncoder(&buf).Encode(pdoc)
			if err != nil {
				return err
			}
		}

		doc := Doc{
			Version: doc.PackageVersion,
			Gob:     buf.Bytes(),
		}
		if _, err := datastore.Put(c, key, &doc); err != nil {
			c.Errorf("Put(%s) -> %v", importPath, err)
		}
	}

	// Update the package index. To minimize datastore costs and cache
	// invalidations, the datastore is conditionally updated by comparing the
	// package to the stored package.

	keyName := importPath
	if pdoc != nil && pdoc.ProjectRoot == "" {
		// Adjust standard package key name.
		keyName = "/" + keyName
	}

	var invalidateCache bool
	key = datastore.NewKey(c, "Package", keyName, 0, nil)
	var storedPackage Package
	err := datastore.Get(c, key, &storedPackage)
	switch err {
	case datastore.ErrNoSuchEntity:
		if pkg != nil {
			invalidateCache = true
			c.Infof("Adding package %s", importPath)
			if _, err := datastore.Put(c, key, pkg); err != nil {
				c.Errorf("Put(%s) -> %v", importPath, err)
			}
		}
	case nil:
		if pkg == nil {
			invalidateCache = true
			c.Infof("Deleting package %s", importPath)
			if err := datastore.Delete(c, key); err != datastore.ErrNoSuchEntity && err != nil {
				c.Errorf("Delete(%s) -> %v", importPath, err)
			}
		} else if !pkg.equal(&storedPackage) {
			invalidateCache = true
			c.Infof("Updating package %s", importPath)
			if _, err := datastore.Put(c, key, pkg); err != nil {
				c.Errorf("Put(%s) -> %v", importPath, err)
			}
		}
	default:
		c.Errorf("Get(%s) -> %v", importPath, err)
	}

	// Update memcache.

	if invalidateCache {
		keys := []string{packageListKey}
		if pdoc != nil {
			keys = append(keys, projectListKeyPrefix+pdoc.ProjectRoot)
		}
		if err = cacheClear(c, keys...); err != nil {
			return err
		}
	}
	return nil
}
