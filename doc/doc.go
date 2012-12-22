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
	"encoding/xml"
	"errors"
	"net/http"
	"path"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// service represents a source code control service.
type service struct {
	pattern *regexp.Regexp
	getDoc  func(*http.Client, []string, string) (*Package, error)
	prefix  string
}

// services is the list of source code control services handled by gopkgdoc.
var services = []*service{
	&service{githubPattern, getGithubDoc, "github.com/"},
	&service{googlePattern, getGoogleDoc, "code.google.com/"},
	&service{bitbucketPattern, getBitbucketDoc, "bitbucket.org/"},
	&service{launchpadPattern, getLaunchpadDoc, "launchpad.net/"},
	&service{gitoriousPattern, getGitoriousDoc, "git.gitorious.org/"},
}

func attrValue(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if strings.EqualFold(a.Name.Local, name) {
			return a.Value
		}
	}
	return ""
}

func getMeta(client *http.Client, importPath string) (projectRoot, projectName, projectURL, repoRoot string, err error) {
	var resp *http.Response

	uri := importPath
	if !strings.Contains(uri, "/") {
		// Add slash for root of domain.
		uri = uri + "/"
	}
	uri = uri + "?go-get=1"

	proto := "https://"
	resp, err = client.Get(proto + uri)
	if err != nil || resp.StatusCode != 200 {
		if err == nil {
			resp.Body.Close()
		}
		proto = "http://"
		resp, err = client.Get(proto + uri)
		if err != nil {
			err = GetError{strings.SplitN(importPath, "/", 2)[0], err}
			return
		}
	}
	defer resp.Body.Close()

	d := xml.NewDecoder(resp.Body)
	d.Strict = false

	err = ErrPackageNotFound
	for {
		t, tokenErr := d.Token()
		if tokenErr != nil {
			break
		}
		switch t := t.(type) {
		case xml.EndElement:
			if strings.EqualFold(t.Name.Local, "head") {
				return
			}
		case xml.StartElement:
			if strings.EqualFold(t.Name.Local, "body") {
				return
			}
			if !strings.EqualFold(t.Name.Local, "meta") ||
				attrValue(t.Attr, "name") != "go-import" {
				continue
			}
			f := strings.Fields(attrValue(t.Attr, "content"))
			if len(f) != 3 ||
				!strings.HasPrefix(importPath, f[0]) ||
				!(len(importPath) == len(f[0]) || importPath[len(f[0])] == '/') {
				continue
			}
			if err == nil {
				// More than one matching meta tag. Handle as not found.
				err = ErrPackageNotFound
				return
			}
			err = nil
			projectRoot = f[0]
			repoRoot = f[2]
			_, projectName = path.Split(projectRoot)
			projectURL = proto + projectRoot
		}
	}
	return
}

// getDynamic gets a document from a service that is not statically known.
func getDynamic(client *http.Client, importPath string, etag string) (*Package, error) {
	projectRoot, projectName, projectURL, repoRoot, err := getMeta(client, importPath)
	if err != nil {
		return nil, err
	}

	if projectRoot != importPath {
		var projectRoot2 string
		projectRoot2, projectName, projectURL, _, err = getMeta(client, projectRoot)
		if err != nil {
			return nil, err
		}
		if projectRoot2 != projectRoot {
			return nil, ErrPackageNotFound
		}
	}

	i := strings.Index(repoRoot, "://")
	if i < 0 {
		return nil, ErrPackageNotFound
	}
	importPath2 := repoRoot[i+len("://"):] + importPath[len(projectRoot):]

	pdoc, err := getStatic(client, importPath2, etag)

	if err == nil {
		pdoc.ImportPath = importPath
		pdoc.ProjectRoot = projectRoot
		pdoc.ProjectName = projectName
		pdoc.ProjectURL = projectURL
		return pdoc, err
	}

	if err == errNoMatch {
		return getProxyDoc(client, importPath, projectRoot, projectName, projectURL, etag)
	}

	return nil, err
}

var errNoMatch = errors.New("no match")

// getStatic gets a document from a statically known service. getStatic returns
// errNoMatch if the import path is not recognized.
func getStatic(client *http.Client, importPath string, etag string) (*Package, error) {
	for _, s := range services {
		if !strings.HasPrefix(importPath, s.prefix) {
			continue
		}
		m := s.pattern.FindStringSubmatch(importPath)
		if m == nil && s.prefix != "" {
			// Import path is bad if prefix matches and regexp does not.
			return nil, ErrPackageNotFound
		}
		return s.getDoc(client, m, etag)
	}
	return nil, errNoMatch
}

func Get(client *http.Client, importPath string, etag string) (pdoc *Package, err error) {

	const versionPrefix = PackageVersion + "-"

	if strings.HasPrefix(etag, versionPrefix) {
		etag = etag[len(versionPrefix):]
	} else {
		etag = ""
	}

	switch {
	case StandardPackages[importPath]:
		pdoc, err = getStandardDoc(client, importPath, etag)
	case !ValidRemotePath(importPath):
		return nil, ErrPackageNotFound
	default:
		pdoc, err = getStatic(client, importPath, etag)
		if err == errNoMatch {
			pdoc, err = getDynamic(client, importPath, etag)
		}
	}

	if err == nil {
		pdoc.Etag = versionPrefix + pdoc.Etag
	}

	return pdoc, err
}

var (
	ErrPackageNotFound    = errors.New("package not found")
	ErrPackageNotModified = errors.New("package not modified")
)

var badTLDs = []string{".png", ".html", ".jpg", ".ico", ".txt", ".xml", ".go", ".gif"}
var validHost = regexp.MustCompile(`^[-A-Za-z0-9]+(?:\.[-A-Za-z0-9]+)+`)

// ValidRemotePath returns true if importPath is structurally valid for "go get".
func ValidRemotePath(importPath string) bool {

	// See isbadimport in $GOROOT/src/cmd/gc/subr.c for rune checks.
	for _, r := range importPath {
		if r == utf8.RuneError {
			return false
		}
		if r < 0x20 || r == 0x7f {
			return false
		}
		if r == '\\' {
			return false
		}
		if unicode.IsSpace(r) {
			return false
		}
		if strings.IndexRune("!\"#$%&'()*,:;<=>?[]^`{|}", r) >= 0 {
			return false
		}
	}

	parts := strings.Split(importPath, "/")
	if len(parts) == 0 {
		return false
	}

	if !validHost.MatchString(parts[0]) {
		return false
	}

	for _, tld := range badTLDs {
		if strings.HasSuffix(parts[0], tld) {
			return false
		}
	}

	for _, part := range parts[1:] {
		if len(part) == 0 ||
			part[0] == '.' ||
			part[0] == '_' ||
			part == "testdata" {
			return false
		}
	}

	return true
}
