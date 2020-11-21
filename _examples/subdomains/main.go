package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/kataras/rewrite"
)

// Router a custom implementation to support root and sub domains.
// It's just an example, you can use whatever router you want,
// e.g. https://github.com/kataras/iris
type Router struct {
	// rootDomain should be the root domain name, including the port,
	// we need that one for our custom subdomain router.
	rootDomain string

	// subdomains holds a subdomain and its mux. Read-only map while serving.
	subdomains map[string]http.Handler

	*http.ServeMux
}

// ServeHTTP completes the http.Handler interface.
func (router *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	subdomain := strings.TrimSuffix(r.Host, router.rootDomain) // including the dot.
	if len(subdomain) > 1 && subdomain != router.rootDomain {
		subdomain = subdomain[0 : len(subdomain)-1] // remove the dot '.'.
		mux, ok := router.subdomains[subdomain]     // serve based on the subdomain part.
		if !ok {
			http.Error(w, "Not found", 404)
			return
		}
		mux.ServeHTTP(w, r)
		return
	}

	// serve root-domain.
	router.ServeMux.ServeHTTP(w, r)
}

// HandleSubdomain sets a subdomain handler.
func (router *Router) HandleSubdomain(subdomain string, mux http.Handler) {
	router.subdomains[subdomain] = mux
}

// newRouter initializes a new router.
func newRouter(rootDomain string) *Router {
	rootMux := http.NewServeMux()

	return &Router{
		rootDomain: rootDomain,
		subdomains: map[string]http.Handler{
			// add www subdomain to point to the root domain automatically
			// because our rewrite middleware redirects all root requests to www.
			"www": rootMux,
		},
		ServeMux: rootMux,
	}
}

func main() {
	router := newRouter("mydomain.com:8080")
	// root domain.
	router.HandleFunc("/", index)
	router.HandleFunc("/about", about)
	router.HandleFunc("/docs", docs)
	router.HandleFunc("/users", listUsers)

	// test subdomain.
	testRouter := http.NewServeMux()
	testRouter.HandleFunc("/", testIndex)
	// newtest subdomain.
	newtestRouter := http.NewServeMux()
	newtestRouter.HandleFunc("/", newTestIndex)
	newtestRouter.HandleFunc("/about", newTestAbout)

	router.HandleSubdomain("test", testRouter)
	router.HandleSubdomain("newtest", newtestRouter)

	/*
		| SOURCE                                   | TARGET                                         |
		|------------------------------------------|------------------------------------------------|
		| http://mydomain.com:8080/seo/about       | http://www.mydomain.com:8080/about             |
		| http://test.mydomain.com:8080            | http://newtest.mydomain.com:8080               |
		| http://test.mydomain.com:8080/seo/about  | http://newtest.mydomain.com:8080/about         |
		| http://mydomain.com:8080/seo             | http://www.mydomain.com:8080                   |
		| http://mydomain.com:8080/about           | http://www.mydomain.com:8080/about             |
		| http://mydomain.com:8080/docs/v12/hello  | http://www.mydomain.com:8080/docs              |
		| http://mydomain.com:8080/docs/v12some    | http://www.mydomain.com:8080/docs              |
		| http://mydomain.com:8080/oldsome         | http://www.mydomain.com:8080                   |
		| http://mydomain.com:8080/oldindex/random | http://www.mydomain.com:8080                   |
		| http://mydomain.com:8080/users.json      | http://www.mydomain.com:8080/users?format=json |
	*/
	redirects := rewrite.Load("redirects.yml")
	log.Println("Listening on :8080")
	http.ListenAndServe(":8080", redirects(router)) // see hosts file too.

	/* OR programmatically:
	opts := rewrite.Options{
		RedirectMatch: []string{
			"301 /seo/(.*) /$1",
			"301 /docs/v12(.*) /docs",
			"301 /old(.*) /",
			"301 ^(http|https)://test.(.*) $1://newtest.$2",
			"0 /(.*).(json|xml) /$1?format=$2",
		},
		PrimarySubdomain: "www",
	}
	engine, err := rewrite.New(opts)
	http.ListenAndServe(":8080",  engine.Handler(router))
	*/
}

func index(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Index")
}

func about(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "About")
}

func docs(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Docs")
}

func listUsers(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "text"
	}
	/*
		switch format{
			case "json":
				JSON response...
			case "xml":
				XML response...
			// [...]
		}
	*/
	fmt.Fprintf(w, "Format: %s\n", format)
}

func testIndex(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, `Test Subdomain Index
					(This should never be executed,
					redirects to newtest subdomain)`)
}

func newTestIndex(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "New Test Subdomain Index")
}

func newTestAbout(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "New Test Subdomain About")
}
