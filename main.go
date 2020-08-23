package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/site/doc"
	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/memcache"
	"google.golang.org/appengine/urlfetch"
)

const (
	configuredHost = "www.gorillatoolkit.org"
	defaultPort    = "8080"
	docKeyPrefix   = "doc-" + doc.PackageVersion + ":"
)

func filterCmds(in []*Package) (out []*Package, cmds []*Package) {
	out = in[0:0]
	for _, pkg := range in {
		if pkg.IsCmd {
			cmds = append(cmds, pkg)
		} else {
			out = append(out, pkg)
		}
	}
	return
}

func childPackages(c context.Context, projectRoot, importPath string) ([]*Package, error) {
	projectPkgs, err := queryPackages(c, projectListKeyPrefix+projectRoot,
		datastore.NewQuery("Package").
			Filter("__key__ >", datastore.NewKey(c, "Package", projectRoot+"/", 0, nil)).
			Filter("__key__ <", datastore.NewKey(c, "Package", projectRoot+"0", 0, nil)))
	if err != nil {
		return nil, err
	}

	prefix := importPath + "/"
	pkgs := projectPkgs[0:0]
	for _, pkg := range projectPkgs {
		if strings.HasPrefix(pkg.ImportPath, prefix) {
			pkgs = append(pkgs, pkg)
		}
	}
	return pkgs, nil
}

// getDoc gets the package documentation and child packages for the given import path.
func getDoc(c context.Context, importPath string) (*doc.Package, []*Package, error) {

	// 1. Look for doc in cache.

	cacheKey := docKeyPrefix + importPath
	var pdoc *doc.Package
	item, err := cacheGet(c, cacheKey, &pdoc)
	switch err {
	case nil:
		pkgs, err := childPackages(c, pdoc.ProjectRoot, importPath)
		if err != nil {
			return nil, nil, err
		}
		return pdoc, pkgs, err
	case memcache.ErrCacheMiss:
		// OK
	default:
		return nil, nil, err
	}

	// 2. Look for doc in store.

	pdocSaved, etag, err := loadDoc(c, importPath)
	if err != nil {
		return nil, nil, err
	}

	// 3. Get documentation from the version control service and update
	// datastore and cache as needed.

	pdoc, err = doc.Get(urlfetch.Client(c), importPath, etag)
	log.Infof(c, "doc.Get(%q, %q) -> %v", importPath, etag, err)

	switch err {
	case nil:
		if err := updatePackage(c, importPath, pdoc); err != nil {
			return nil, nil, err
		}
		item.Object = pdoc
		item.Expiration = time.Hour
		if err := cacheSet(c, item); err != nil {
			return nil, nil, err
		}
	case doc.ErrPackageNotFound:
		if err := updatePackage(c, importPath, nil); err != nil {
			return nil, nil, err
		}
		return nil, nil, doc.ErrPackageNotFound
	case doc.ErrPackageNotModified:
		pdoc = pdocSaved
	default:
		if pdocSaved == nil {
			return nil, nil, err
		}
		log.Errorf(c, "Serving %s from store after error from VCS.", importPath)
		pdoc = pdocSaved
	}

	// 4. Find the child packages.

	pkgs, err := childPackages(c, pdoc.ProjectRoot, importPath)
	if err != nil {
		return nil, nil, err
	}

	// 5. Convert to not found if package is empty.

	if len(pkgs) == 0 && pdoc.Name == "" && len(pdoc.Errors) == 0 {
		return nil, nil, doc.ErrPackageNotFound
	}

	// 6. Done

	return pdoc, pkgs, nil
}

// handlerFunc adapts a function to an http.Handler.
type handlerFunc func(http.ResponseWriter, *http.Request) error

func (f handlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Host != configuredHost && !appengine.IsDevAppServer() {
		r.URL.Host = configuredHost
		http.Redirect(w, r, r.URL.String(), 301)
		return
	}

	err := f(w, r)
	if err != nil {
		if e, ok := err.(doc.GetError); ok {
			http.Error(w, "Error getting files from "+e.Host+".", http.StatusInternalServerError)
		} else if appengine.IsOverQuota(err) {
			http.Error(w, "Internal error: "+err.Error(), http.StatusInternalServerError)
		} else {
			http.Error(w, "Internal Error", http.StatusInternalServerError)
		}
	}
}

func notFoundHandler(w http.ResponseWriter, r *http.Request) error {
	return executeTemplate(w, "404", 200, nil)
}

func homeHandler(w http.ResponseWriter, r *http.Request) error {
	return executeTemplate(w, "home", 200, map[string]interface{}{
		"Section": "home",
	})
}

func peopleHandler(w http.ResponseWriter, r *http.Request) error {
	return executeTemplate(w, "people", 200, map[string]interface{}{
		"Section": "people",
	})
}

func packageIndexHandler(w http.ResponseWriter, r *http.Request) error {
	c := appengine.NewContext(r)
	pkgs, err := queryPackages(c, packageListKey, datastore.NewQuery("Package").Filter("Hide=", false))
	if err != nil {
		return err
	}
	pkgs, cmds := filterCmds(pkgs)
	return executeTemplate(w, "pkgIndex", 200, map[string]interface{}{
		"Section": "pkg",
		"pkgs":    pkgs,
		"cmds":    cmds,
	})
}

func packageHandler(w http.ResponseWriter, r *http.Request) error {
	importPath := fullImportPath(mux.Vars(r)["package"])
	c := appengine.NewContext(r)
	pdoc, pkgs, err := getDoc(c, importPath)
	switch err {
	case doc.ErrPackageNotFound:
		return executeTemplate(w, "404", 404, nil)
	case nil:
		//ok
	default:
		return err
	}
	pkgs, cmds := filterCmds(pkgs)
	return executeTemplate(w, "pkg", 200, map[string]interface{}{
		"Section": "pkg",
		"pkgs":    pkgs,
		"cmds":    cmds,
		"pdoc":    pdoc,
	})
}

func packageReloadHandler(w http.ResponseWriter, r *http.Request) error {
	c := appengine.NewContext(r)
	importPath := r.FormValue("importPath")
	cacheKey := docKeyPrefix + importPath
	err := memcache.Delete(c, cacheKey)
	log.Infof(c, "memcache.Delete(%s) -> %v\n", cacheKey, err)
	removeDoc(c, importPath)
	http.Redirect(w, r, "/"+importPath, 302)
	return nil
}

func sourceIndexHandler(w http.ResponseWriter, r *http.Request) error {
	return executeTemplate(w, "srcIndex", 200, map[string]interface{}{
		"Section": "src",
	})
}

func sourceHandler(w http.ResponseWriter, r *http.Request) error {
	return executeTemplate(w, "src", 200, map[string]interface{}{
		"Section": "src",
	})
}

func fullImportPath(importPath string) string {
	return "github.com/gorilla/" + importPath
}

func main() {
	var port string
	if port = os.Getenv("PORT"); port == "" {
		port = defaultPort
	}

	r := mux.NewRouter()

	packageGorillaHandler := func(w http.ResponseWriter, req *http.Request) error {
		vars := mux.Vars(req)
		u, err := r.Get("package").URL("package", vars["package"])
		if err != nil {
			return err
		}
		http.Redirect(w, req, u.String(), 301)
		return nil
	}

	r.StrictSlash(true)
	r.Handle("/", handlerFunc(homeHandler))
	r.Handle("/people", handlerFunc(peopleHandler))
	r.Handle("/pkg/", handlerFunc(packageIndexHandler))
	r.Handle("/pkg/gorilla/{package:.*}", handlerFunc(packageGorillaHandler))
	r.Handle("/pkg/{package:.*}", handlerFunc(packageHandler)).Name("package")
	r.Handle("/src/", handlerFunc(sourceIndexHandler))
	r.Handle("/src/{file:.*}", handlerFunc(sourceHandler))
	r.Handle("/{path:.*}", handlerFunc(notFoundHandler))

	if appengine.IsAppEngine() {
		http.Handle("/", r)
		appengine.Main()
	} else {
		if err := http.ListenAndServe(fmt.Sprintf(":%s", port), r); err != nil {
			fmt.Printf("Error: %v", err.Error())
		}
	}
}
