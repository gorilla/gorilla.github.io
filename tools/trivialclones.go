// Find and print import paths of trivial Github clones.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
)

func get(urlStr string) ([]byte, error) {
	resp, err := http.Get(urlStr)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GET %s returned status %d", urlStr, resp.StatusCode)
	}
	p, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func getJSON(urlStr string, value interface{}) error {
	p, err := get(urlStr)
	if err != nil {
		return err
	}
	return json.Unmarshal(p, value)
}

func main() {
	out := bufio.NewWriter(os.Stdout)
	p, err := get("http://go.pkgdoc.org/a/index")
	if err != nil {
		log.Fatal(err)
	}
	pat := regexp.MustCompile(`^github\.com/([^/]+/[^/]+)`)
	trivial := make(map[string]bool)
	for _, b := range bytes.Split(p, []byte{'\n'}) {
		importPath := string(bytes.TrimSpace(b))
		m := pat.FindStringSubmatch(importPath)
		if m == nil {
			continue
		}
		userRepo := m[1]
		if t, ok := trivial[userRepo]; ok {
			if t {
				out.WriteString(importPath)
				out.WriteByte('\n')
			}
			continue
		}
		trivial[userRepo] = false
		var repo struct {
			Source struct {
				URL string `json:"url"`
			} `json:"source"`
		}
		err = getJSON("https://api.github.com/repos/"+userRepo, &repo)
		if err != nil {
			log.Print(err)
			continue
		}
		if repo.Source.URL == "" {
			continue
		}
		var ref struct {
			Object struct {
				SHA string `json:"sha"`
			} `json:"object"`
		}
		err = getJSON("https://api.github.com/repos/"+userRepo+"/git/refs/heads/master", &ref)
		if err != nil {
			log.Print(err)
			continue
		}

		var commit struct {
			Message string `json:"message"`
		}

		err = getJSON(repo.Source.URL+"/git/commits/"+ref.Object.SHA, &commit)
		if err == nil {
			out.WriteString(importPath)
			out.WriteByte('\n')
			trivial[userRepo] = true
		}
	}
	out.Flush()
}
