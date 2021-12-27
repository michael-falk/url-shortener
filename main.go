package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/gorilla/mux"
)

type NewShortLinkRequest struct {
	Url      string  `json:"url"`
	ExpireAt *string `json:"expireAt,omitempty"` // RFC3339 datetime
}

// From https://golangdocs.com/generate-random-string-in-golang
// Decided for case-insensitivity so lowercase only
func RandomString(n int) string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyz0123456789")

	s := make([]rune, n)
	for i := range s {
		s[i] = letters[rand.Intn(len(letters))]
	}
	return string(s)
}

func CreateNamedShortLink(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["short_link"]

	var req NewShortLinkRequest
	json.NewDecoder(r.Body).Decode(&req)

	HandleNewShortLinkRequest(id, req, w)
}

func CreateRandomShortLink(w http.ResponseWriter, r *http.Request) {
	var req NewShortLinkRequest
	json.NewDecoder(r.Body).Decode(&req)

	id := RandomString(6) // TODO make configurable

	HandleNewShortLinkRequest(id, req, w)
}

func HandleNewShortLinkRequest(id string, req NewShortLinkRequest, w http.ResponseWriter) {
	if req.ExpireAt == nil {
		fmt.Fprintf(w, "Created short link: %s for long url: %s\n", id, req.Url)
	} else {
		fmt.Fprintf(w, "Created short link: %s for long url: %s and expiryAt: %s\n", id, req.Url, *req.ExpireAt)
	}
}

func DeleteShortLink(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["short_link"]

	fmt.Fprintf(w, "Deleted: %s\n", id)
}

func GetAnalyticsForShortLink(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["short_link"]
	ago := vars["ago"] //ago=[24h,7d]

	switch {
	case ago == "all":
		fmt.Fprintf(w, "Retrieved analytics for shortlink: %s for all time\n", id)
	case ago == "24h":
		fmt.Fprintf(w, "Retrieved analytics for shortlink: %s for last 24 hours\n", id)
	case ago == "7d":
		fmt.Fprintf(w, "Retrieved analytics for shortlink: %s for last 7 days\n", id)
	default:
		fmt.Fprintf(w, "Unsupporter lookback for %s. Rejecting...\n", id)
	}
}

// From https://hackernoon.com/writing-a-reverse-proxy-in-just-one-line-with-go-c1edfa78c84b
func proxy(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["short_link"]
	url, _ := url.Parse(lookup(id))

	r.URL.Path = ""
	r.URL.Host = url.Host
	r.URL.Scheme = url.Scheme
	r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
	r.Host = url.Host

	// Appears to append a trailing slash because of https://github.com/golang/go/pull/50339.
	// Not sure of workaround. Will comment as limitation and demonstrate with sites that support trailing slash
	httputil.NewSingleHostReverseProxy(url).ServeHTTP(w, r)
}

// lookup the long url for a given id/short_link
func lookup(id string) string {
	fmt.Println(id)
	// return "http://google.com"
	return "https://www.reddit.com/r/golang/comments/7174sr/how_to_remove_the_trailing_slash_in_a_path/"
}

func main() {
	router := mux.NewRouter()

	shortLinkRouter := router.PathPrefix("/v1/admin/short-link").Subrouter()
	shortLinkRouter.HandleFunc("", CreateRandomShortLink).Methods("POST")
	shortLinkRouter.HandleFunc("/{short_link}", CreateNamedShortLink).Methods("POST")
	shortLinkRouter.HandleFunc("/{short_link}", DeleteShortLink).Methods("DELETE")
	shortLinkRouter.HandleFunc("/{short_link}/analytics", GetAnalyticsForShortLink).Methods("GET").Queries("ago", "{ago}")
	shortLinkRouter.HandleFunc("/{short_link}/analytics", GetAnalyticsForShortLink).Methods("GET")

	router.HandleFunc("/s/{short_link}", proxy).Methods("GET")

	// TODO auth so that owning engineering team will have permissions to call the WRITE admin route for the short link. Using JWT semantics. Will not verify tokens. just check `sub` to identify tenant for short-links.

	// DB schema: I want to learn postgres. Also probably good enough for a local fake service. I guess redis cache could be used but over-engineering for a docker-compose local app
	// Table: URLS PK: short_link (hash), owner, destination
	// Usage: PK: id, int (auto-increment) ((Some sortable key. Cannot rely on uniqueness of timestamp because of possible collision or heavy reliance on precision)), FK short_link, datetime (indexed sort)
	// Maybe use timescale DB for usage DB but deffo overkill, right?
	// Usage table will support analytics queries. Append on each forwarded request. Do COUNT where DATE >= AGO(24h, 7d) OR just count if no arguments

	// Metrics: Probably easier to work with than self-managed analytics endpoint... BUT (probably) not as durable than postgres, etc.

	// write unit tests and e2e script after docker-compose implemented. Maybe embed e2e script as extended part of docker-compose

	http.ListenAndServe(":80", router)
}
