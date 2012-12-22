package doc

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"path"
	"strings"
)

// normalizeDir removes leading slash and adds trailing slash.
func normalizeDir(s string) string {
	if len(s) > 0 && s[0] == '/' {
		s = s[1:] + "/"
	}
	return s
}

// isDocFile returns true if a file with the path p should be included in the
// documentation.
func isDocFile(p string) bool {
	_, n := path.Split(p)
	return strings.HasSuffix(n, ".go") && len(n) > 0 && n[0] != '_' && n[0] != '.'
}

type GetError struct {
	Host string
	err  error
}

func (e GetError) Error() string {
	return e.err.Error()
}

// fetchFiles fetches the source files specified by the rawURL field in parallel.
func fetchFiles(client *http.Client, files []*source, header http.Header) error {
	ch := make(chan error)
	for i := range files {
		go func(i int) {
			req, err := http.NewRequest("GET", files[i].rawURL, nil)
			if err != nil {
				ch <- err
				return
			}
			for k, vs := range header {
				req.Header[k] = vs
			}
			resp, err := client.Do(req)
			if err != nil {
				ch <- GetError{req.URL.Host, err}
				return
			}
			if resp.StatusCode != 200 {
				ch <- GetError{req.URL.Host, fmt.Errorf("get %s -> %d", req.URL, resp.StatusCode)}
				return
			}
			files[i].data, err = ioutil.ReadAll(resp.Body)
			if err != nil {
				ch <- GetError{req.URL.Host, err}
				return
			}
			ch <- nil
		}(i)
	}
	for _ = range files {
		if err := <-ch; err != nil {
			return err
		}
	}
	return nil
}

// httpGet gets the specified resource. ErrPackageNotFound is returned if the
// server responds with status 404.
func httpGet(client *http.Client, url string) (io.ReadCloser, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, GetError{req.URL.Host, err}
	}
	if resp.StatusCode == 200 {
		return resp.Body, nil
	}
	resp.Body.Close()
	if resp.StatusCode == 404 {
		err = ErrPackageNotFound
	} else {
		err = GetError{req.URL.Host, fmt.Errorf("get %s -> %d", url, resp.StatusCode)}
	}
	return nil, err
}

// httpGet gets the specified resource. ErrPackageNotFound is returned if the
// server responds with status 404.
func httpGetBytes(client *http.Client, url string) ([]byte, error) {
	rc, err := httpGet(client, url)
	if err != nil {
		return nil, err
	}
	p, err := ioutil.ReadAll(rc)
	rc.Close()
	return p, err
}

// httpGetBytesNoneMatch conditionally gets the specified resource. If a 304 status
// is returned, then the function returns ErrPackageNotModified. If a 404
// status is returned, then the function returns ErrPackageNotFound. 
func httpGetBytesNoneMatch(client *http.Client, url string, etag string) ([]byte, string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("If-None-Match", `"`+etag+`"`)
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", GetError{req.URL.Host, err}
	}
	defer resp.Body.Close()

	etag = resp.Header.Get("Etag")
	if len(etag) >= 2 && etag[0] == '"' && etag[len(etag)-1] == '"' {
		etag = etag[1 : len(etag)-1]
	} else {
		etag = ""
	}

	switch resp.StatusCode {
	case 200:
		p, err := ioutil.ReadAll(resp.Body)
		return p, etag, err
	case 404:
		return nil, "", ErrPackageNotFound
	case 304:
		return nil, "", ErrPackageNotModified
	default:
		return nil, "", GetError{req.URL.Host, fmt.Errorf("get %s -> %d", url, resp.StatusCode)}
	}
	panic("unreachable")
}

// httpGet gets the specified resource. ErrPackageNotFound is returned if the
// server responds with status 404. ErrPackageNotModified is returned if the
// hash of the resource equals savedEtag.
func httpGetBytesCompare(client *http.Client, url string, savedEtag string) ([]byte, string, error) {
	p, err := httpGetBytes(client, url)
	if err != nil {
		return nil, "", err
	}
	h := md5.New()
	h.Write(p)
	etag := hex.EncodeToString(h.Sum(nil))
	if savedEtag == etag {
		err = ErrPackageNotModified
	}
	return p, etag, err
}
