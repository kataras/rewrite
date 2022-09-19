# Rewrite

[![build status](https://img.shields.io/github/workflow/status/kataras/rewrite/CI/master?style=for-the-badge)](https://github.com/kataras/rewrite/actions) [![report card](https://img.shields.io/badge/report%20card-a%2B-ff3333.svg?style=for-the-badge)](https://goreportcard.com/report/github.com/kataras/rewrite) [![godocs](https://img.shields.io/badge/go-%20docs-488AC7.svg?style=for-the-badge)](https://pkg.go.dev/github.com/kataras/rewrite)

Like [Apache mod_rewrite](https://httpd.apache.org/docs/2.4/rewrite/) but for Golang's [net/http](https://golang.org/pkg/net/http/). Initially created for the [Iris Web Framework](https://github.com/kataras/iris) a long time ago. The success of its usefulness is well known as many others have copied and moved the [original source code](https://github.com/kataras/iris/tree/master/middleware/rewrite) into various frameworks since then, if you deem it necessary, you are free to do the same.

## Installation

The only requirement is the [Go Programming Language](https://golang.org/dl).

```sh
$ go get github.com/kataras/rewrite
```

### Examples

- [Basic](_examples/basic)
- [Subdomains](_examples/subdomains)

## Getting Started

The Rewrite Middleware supports rewrite URL path, subdomain or host based on a regular expression search and replace.

The syntax is familiar to the majority of the backend developers out there and it looks like that:

| REDIRECT_CODE_DIGITS | PATTERN_REGEX | TARGET_REPL |
|----------------------|---------------|-------------|
| 301                  | /seo/(.*)     | /$1         |

Would redirect all requests from relative path `/seo/*` to `/*` using the `301 (Moved Permanently)` HTTP Status Code. Learn more about [regex](https://golang.org/pkg/regexp/#example_Regexp_ReplaceAllString).

**Usage**

First of all, you should import the builtin middleware as follows:

```go
import "github.com/kataras/rewrite"
```

There are two ways to load rewrite options in order to parse and register the redirect rules:

**1.** Through code using the `New` function and `Handler` method. Parse errors can be handled and rules can be programmatically created.

```go
// 1. Code the redirect rules.
opts := rewrite.Options{
	RedirectMatch: []string{
		"301 /seo/(.*) /$1",
		"301 /docs/v12(.*) /docs",
		"301 /old(.*) /",
	},
	PrimarySubdomain: "www",
}
// 2. Initialize the Rewrite Engine.
rw, err := rewrite.New(opts)
if err != nil { 
	panic(err)
}

// 3. Wrap the router using Engine's Handler method.
http.ListenAndServe(":8080", rw.Handler(mux))
```

**2.** Or through a `yaml` file using the `Load` function which returns a `func(http.Handler) http.Handler`. It is the most common scenario and the simplest one. It panics on parse errors.

The `"redirects.yml"` file looks like that:

```yaml
RedirectMatch:
  # Redirects /seo/* to /*
  - 301 /seo/(.*) /$1

  # Redirects /docs/v12* to /docs
  - 301 /docs/v12(.*) /docs

  # Redirects /old(.*) to /
  - 301 /old(.*) /

# Redirects root domain requests to www.
PrimarySubdomain: www
```

```go
func main() {
    mux := http.NewServeMux()
    // [...routes]
	redirects := rewrite.Load("redirects.yml")
	// Wrap the router.
    http.ListenAndServe(":8080", redirects(mux))
}
```

## Example

Let's write a simple application which follows the redirect rules of:

| SOURCE                                   | TARGET                             |
|------------------------------------------|------------------------------------|
| http://localhost:8080/seo/about          | http://localhost:8080/about        |
| http://mydomain.com:8080/docs/v12/hello  | http://www.mydomain.com:8080/docs  |
| http://mydomain.com:8080/docs/v12some    | http://www.mydomain.com:8080/docs  |
| http://mydomain.com:8080/oldsome         | http://www.mydomain.com:8080       |
| http://mydomain.com:8080/oldindex/random | http://www.mydomain.com:8080       |

### Server

```go
package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/kataras/rewrite"
)

func main() {
	// Code the redirect rules.
	opts := rewrite.Options{
		RedirectMatch: []string{
			"301 /seo/(.*) /$1",       // redirect /seo/* to /*
			"301 /docs/v12(.*) /docs", // redirect docs/v12/* to /docs
			"301 /old(.*) /",          // redirect /old** to /
		},
		PrimarySubdomain: "www", // redirect root to www. subdomain.
	}
	// Initialize the Rewrite Engine.
	rw, err := rewrite.New(opts)
	if err != nil {
		log.Fatal(err)
	}

	router := http.NewServeMux()
	router.HandleFunc("/", index)
	router.HandleFunc("/about", about)
	router.HandleFunc("/docs", docs)

	log.Println("Listening on :8080")
	// Wrap the router using the Handler method.
	http.ListenAndServe(":8080", rw.Handler(router))
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
```

### Hosts File

```text
127.0.0.1	mydomain.com
127.0.0.1	www.mydomain.com
```

Navigate [here](https://support.rackspace.com/how-to/modify-your-hosts-file/) if you don't know how to modify the system's hosts file.

## License

This software is licensed under the [MIT License](LICENSE).
