package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/kataras/rewrite"
)

func main() {
	opts := rewrite.Options{
		RedirectMatch: []string{
			"301 /seo/(.*) /$1",       // redirect /seo/* to /*
			"301 /docs/v12(.*) /docs", // redirect docs/v12/* to /docs
			"301 /old(.*) /",          // redirect /old** to /
		},
		PrimarySubdomain: "www", // redirect root to www. subdomain.
	}
	rw, err := rewrite.New(opts)
	if err != nil {
		log.Fatal(err)
	}

	router := http.NewServeMux()
	router.HandleFunc("/", index)
	router.HandleFunc("/about", about)
	router.HandleFunc("/docs", docs)

	log.Println("Listening on :8080")
	/*
		| SOURCE                                   | TARGET                             |
		|------------------------------------------|------------------------------------|
		| http://localhost:8080/seo/about          | http://localhost:8080/about        |
		| http://mydomain.com:8080/docs/v12/hello  | http://www.mydomain.com:8080/docs  |
		| http://mydomain.com:8080/docs/v12some    | http://www.mydomain.com:8080/docs  |
		| http://mydomain.com:8080/oldsome         | http://www.mydomain.com:8080       |
		| http://mydomain.com:8080/oldindex/random | http://www.mydomain.com:8080       |
	*/
	http.ListenAndServe(":8080", rw.Handler(router)) // wrap the router, see hosts file too.
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
