package rewrite

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/publicsuffix"
	"gopkg.in/yaml.v3"
)

// Options holds the developer input to customize
// the redirects for the Rewrite Engine.
// See examples for more.
// Look the `New` and `Load` package-level functions too.
type Options struct {
	// RedirectMatch accepts a slice of lines
	// of form:
	// REDIRECT_CODE PATH_PATTERN TARGET_PATH
	// Example: []{"301 /seo/(.*) /$1"}.
	RedirectMatch []string `json:"redirectMatch" yaml:"RedirectMatch"`

	// Root domain requests redirect automatically to primary subdomain.
	// Example: "www" to redirect always to www.
	// Note that you SHOULD NOT create a www subdomain inside the Iris Application.
	// This field takes care of it for you, the root application instance
	// will be used to serve the requests.
	PrimarySubdomain string `json:"primarySubdomain" yaml:"PrimarySubdomain"`

	// Debug to enable debug log.Printf messages.
	Debug bool `json:"debug" yaml:"Debug"`
}

// LoadOptions loads rewrite Options from a system file.
func LoadOptions(filename string) (Options, error) {
	var opts Options

	ext := ".yml"
	if index := strings.LastIndexByte(filename, '.'); index > 1 && len(filename)-1 > index {
		ext = filename[index:]
	}

	f, err := os.Open(filename)
	if err != nil {
		return opts, fmt.Errorf("rewrite: %w", err)
	}
	defer f.Close()

	switch ext {
	case ".yaml", ".yml":
		err = yaml.NewDecoder(f).Decode(&opts)
	case ".json":
		err = json.NewDecoder(f).Decode(&opts)
	default:
		return opts, fmt.Errorf("rewrite: unexpected file extension: %q", filename)
	}

	if err != nil {
		return opts, fmt.Errorf("rewrite: decode file: %q: %w", filename, err)
	}

	return opts, nil
}

// Engine is the rewrite engine main structure.
// Navigate through https://github.com/kataras/rewrite/tree/main/_examples for more.
type Engine struct {
	redirects []*redirectMatch
	options   Options

	logger          *log.Logger
	domainValidator func(string) bool
}

// New returns a new Rewrite Engine based on "opts".
// It reports any parser error.
// See its `Handler` method to register it on an application.
func New(opts Options) (*Engine, error) {
	redirects := make([]*redirectMatch, 0, len(opts.RedirectMatch))

	for _, line := range opts.RedirectMatch {
		r, err := parseRedirectMatchLine(line)
		if err != nil {
			return nil, err
		}
		redirects = append(redirects, r)
	}

	if opts.PrimarySubdomain != "" && !strings.HasSuffix(opts.PrimarySubdomain, ".") {
		opts.PrimarySubdomain += "." // www -> www.
	}

	e := &Engine{
		options:   opts,
		redirects: redirects,
		domainValidator: func(root string) bool {
			return !strings.HasSuffix(root, localhost)
		},
		logger: log.New(os.Stderr, "rewrite: ", log.LstdFlags),
	}
	return e, nil
}

// Load decodes the "filename" options
// and returns a new Rewrite Engine http.Handler wrapper
// that can be used as a middleware.
// It panics on errors.
//
// Usage:
//
//	redirects := Load("redirects.yml")
//	http.ListenAndServe(":8080", redirects(router))
//
// See `New` package-level function too.
func Load(filename string) func(http.Handler) http.Handler {
	opts, err := LoadOptions(filename)
	if err != nil {
		panic(err)
	}
	engine, err := New(opts)
	if err != nil {
		panic(err)
	}
	return engine.Handler
}

// SetLogger attachs a logger to the Rewrite Engine,
// used only for debugging.
func (e *Engine) SetLogger(logger *log.Logger) *Engine {
	e.logger = logger
	return e
}

func (e *Engine) debugf(format string, args ...interface{}) {
	if e.options.Debug {
		e.logger.Printf(format, args...)
	}
}

// Handler is the main Engine's method.
// It is http.Handler wrapper that should be registered at the
// top of the http application server.
// Usage:
//
//	http.ListenAndServe(":8080", rw.Handler(router))
func (e *Engine) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e.rewrite(w, r, next)
	})
}

const localhost = "localhost"

func (e *Engine) rewrite(w http.ResponseWriter, r *http.Request, next http.Handler) {
	if primarySubdomain := e.options.PrimarySubdomain; primarySubdomain != "" {
		hostport := getHost(r)
		root := getDomain(hostport)

		e.debugf("Begin request: full host: %s and root domain: %s", hostport, root)
		// Note:
		// localhost and 127.0.0.1 are not supported for subdomain rewrite, by purpose,
		// use a virtual host instead.
		// GetDomain will return will return localhost or www.localhost
		// on expected loopbacks.
		if e.domainValidator(root) {
			root += getPort(hostport)
			subdomain := strings.TrimSuffix(hostport, root)

			e.debugf("* Domain is not a loopback, requested subdomain: %s\n", subdomain)

			if subdomain == "" {
				// we are in root domain, full redirect to its primary subdomain.
				newHost := primarySubdomain + root
				e.debugf("* Redirecting from root domain to: %s\n", newHost)
				r.Host = newHost
				r.URL.Host = newHost
				http.Redirect(w, r, r.URL.String(), http.StatusMovedPermanently)
				return
			}

			if subdomain == primarySubdomain {
				// keep root domain as the Host field inside the next handlers,
				// for consistently use and
				// to bypass the subdomain router (`routeHandler`)
				// do not return, redirects should be respected.
				rootHost := strings.TrimPrefix(hostport, subdomain)
				e.debugf("* Request host field was modified to: %s. Proceed without redirection\n", rootHost)
				// modify those for the next redirects or the route handler.
				r.Host = rootHost
				r.URL.Host = rootHost
			}

			// maybe other subdomain or not at all, let's continue.
		} else {
			e.debugf("* Primary subdomain is: %s but redirect response was not sent. Domain is a loopback?\n", primarySubdomain)
		}
	}

	for _, rd := range e.redirects {
		src := r.URL.Path
		if !rd.isRelativePattern {
			// don't change the request, use a full redirect.
			src = getScheme(r) + getHost(r) + r.URL.RequestURI()
		}

		if target, ok := rd.matchAndReplace(src); ok {
			if target == src {
				e.debugf("* WARNING: source and target URLs match: %s\n", src)
				next.ServeHTTP(w, r)
				return
			}

			if rd.noRedirect {
				u, err := r.URL.Parse(target)
				if err != nil {
					http.Error(w, err.Error(), http.StatusMisdirectedRequest)
					return
				}

				e.debugf("* No redirect: handle request: %s as: %s\n", r.RequestURI, u)
				r.URL = u
				next.ServeHTTP(w, r)
				return
			}

			if !rd.isRelativePattern {
				// this performs better, no need to check query or host,
				// the uri already built.
				e.debugf("* Full redirect: from: %s to: %s\n", src, target)
				redirectAbs(w, r, target, rd.code)
			} else {
				e.debugf("Path redirect: from: %s to: %s\n", src, target)
				http.Redirect(w, r, target, rd.code)
			}

			return
		}
	}

	next.ServeHTTP(w, r)
}

type redirectMatch struct {
	code    int
	pattern *regexp.Regexp
	target  string

	isRelativePattern bool
	noRedirect        bool
}

func (r *redirectMatch) matchAndReplace(src string) (string, bool) {
	if r.pattern.MatchString(src) {
		if match := r.pattern.ReplaceAllString(src, r.target); match != "" {
			return match, true
		}
	}

	return "", false
}

func parseRedirectMatchLine(s string) (*redirectMatch, error) {
	parts := strings.Split(strings.TrimSpace(s), " ")
	if len(parts) != 3 {
		return nil, fmt.Errorf("redirect match: invalid line: %s", s)
	}

	codeStr, pattern, target := parts[0], parts[1], parts[2]

	for i, ch := range codeStr {
		if !isDigit(ch) {
			return nil, fmt.Errorf("redirect match: status code digits: %s [%d:%c]", codeStr, i, ch)
		}
	}

	code, err := strconv.Atoi(codeStr)
	if err != nil {
		// this should not happen, we check abt digit
		// and correctly position the error too but handle it.
		return nil, fmt.Errorf("redirect match: status code digits: %s: %v", codeStr, err)
	}

	regex := regexp.MustCompile(pattern)
	if regex.MatchString(target) {
		return nil, fmt.Errorf("redirect match: loop detected: pattern: %s vs target: %s", pattern, target)
	}

	v := &redirectMatch{
		code:              code,
		pattern:           regex,
		target:            target,
		noRedirect:        code <= 0,
		isRelativePattern: pattern[0] == '/', // search by path.
	}

	return v, nil
}

func isDigit(ch rune) bool {
	return '0' <= ch && ch <= '9'
}

const (
	sufscheme   = "://"
	schemeHTTPS = "https"
	schemeHTTP  = "http"
)

// getScheme returns the full scheme of the request URL (https:// or http://).
func getScheme(r *http.Request) string {
	scheme := r.URL.Scheme
	if scheme == "" {
		if r.TLS != nil {
			scheme = schemeHTTPS
		} else {
			scheme = schemeHTTP
		}
	}

	return scheme + sufscheme
}

func getPort(hostport string) string { // returns :port, note that this is only called on non-loopbacks.
	if portIdx := strings.IndexByte(hostport, ':'); portIdx > 0 {
		return hostport[portIdx:]
	}

	return ""
}

func getHost(r *http.Request) string {
	if host := r.URL.Host; host != "" {
		return host
	}
	return r.Host
}

func getDomain(hostport string) string {
	host := hostport
	if tmp, _, err := net.SplitHostPort(hostport); err == nil {
		host = tmp
	}

	switch host {
	case "localhost", "127.0.0.1", "0.0.0.0", "::1", "[::1]", "0:0:0:0:0:0:0:0", "0:0:0:0:0:0:0:1":
		// loopback.
		return "localhost"
	default:
		if domain, err := publicsuffix.EffectiveTLDPlusOne(host); err == nil {
			host = domain
		}

		return host
	}
}

const contentTypeHeaderKey = "Content-Type"

func redirectAbs(w http.ResponseWriter, r *http.Request, url string, code int) {
	// part of net/http std library.
	h := w.Header()

	_, hadCT := h[contentTypeHeaderKey]

	h.Set("Location", url)
	if !hadCT && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
		h.Set(contentTypeHeaderKey, "text/html; charset=utf-8")
	}
	w.WriteHeader(code)

	// Shouldn't send the body for POST or HEAD; that leaves GET.
	if !hadCT && r.Method == http.MethodGet {
		body := "<a href=\"" + template.HTMLEscapeString(url) + "\">" + http.StatusText(code) + "</a>.\n"
		fmt.Fprintln(w, body)
	}
}
