package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"

	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v4/pgxpool"
)

type NewShortLinkRequest struct {
	Url      string     `json:"url"`
	ExpireAt *time.Time `json:"expireAt,omitempty"` // RFC3339 datetime
}

// From https://golangdocs.com/generate-random-string-in-golang
// Decided for case-insensitivity so lowercase only
func RandomString(n int) string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyz0123456789")
	r := rand.New(rand.NewSource(time.Now().UnixMilli()))
	s := make([]rune, n)
	for i := range s {
		s[i] = letters[r.Intn(len(letters))]
	}
	return string(s)
}

// TODO auth so that owning engineering tenant will have permissions to call the WRITE admin route for the short link. Using JWT semantics. Will not verify tokens. just check `sub` to identify tenant for short-links.
// For now, cheat and make a custom X-SUBJECT header to pass in tenant information, etc.

func CreateNamedShortLink(conn *pgxpool.Pool) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["short_link"]
		tenant := r.Header.Get("X-SUBJECT")

		var req NewShortLinkRequest
		json.NewDecoder(r.Body).Decode(&req)

		HandleNewShortLinkRequest(conn, tenant, id, req, w)
	}
}

func CreateRandomShortLink(conn *pgxpool.Pool) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var req NewShortLinkRequest
		json.NewDecoder(r.Body).Decode(&req)
		tenant := r.Header.Get("X-SUBJECT")

		id := RandomString(6) // TODO make configurable

		HandleNewShortLinkRequest(conn, tenant, id, req, w)
	}
}

func HandleNewShortLinkRequest(conn *pgxpool.Pool, tenant, id string, req NewShortLinkRequest, w http.ResponseWriter) {
	if tenant == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Missing required header: X-SUBJECT")
		return
	}

	_, err := url.Parse(req.Url)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprintf(w, "Invalid destination URL")
		return
	}

	if req.ExpireAt == nil {
		_, err = conn.Exec(context.Background(), "INSERT INTO urls (short_link, tenant, destination) VALUES ($1, $2, $3)", id, tenant, req.Url)
	} else {
		_, err = conn.Exec(context.Background(), "INSERT INTO urls (short_link, tenant, destination, expiry) VALUES ($1, $2, $3, $4)", id, tenant, req.Url, req.ExpireAt)
	}

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	resp := make(map[string]string)
	resp["shortLink"] = id
	jsonResp, _ := json.Marshal(resp)
	w.Write(jsonResp)
}

func DeleteShortLink(conn *pgxpool.Pool) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["short_link"]
		tenant := r.Header.Get("X-SUBJECT")

		_, err := conn.Exec(context.Background(), "DELETE FROM urls WHERE short_link=$1 AND tenant=$2", id, tenant)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func GetAllAnalyticsForShortLink(conn *pgxpool.Pool) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["short_link"]
		var count int64
		err := conn.QueryRow(context.Background(), "SELECT COUNT(id) FROM usage WHERE short_link=$1", id).Scan(&count)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
		}

		fmt.Fprintf(w, "%s has been called %d times for all time\n", id, count)

	}
}

func GetAnalyticsForShortLink(conn *pgxpool.Pool, interval string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["short_link"]
		var count int64
		err := conn.QueryRow(context.Background(), "SELECT COUNT(id) FROM usage WHERE short_link=$1"+fmt.Sprintf("AND occurred_at > NOW() - INTERVAL '%s'", interval), id).Scan(&count)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
		}

		fmt.Fprintf(w, "%s has been called %d times in %s\n", id, count, interval)
	}
}

// From https://hackernoon.com/writing-a-reverse-proxy-in-just-one-line-with-go-c1edfa78c84b
func proxy(conn *pgxpool.Pool) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["short_link"]

		urlString, err := lookup(conn, id)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		url, _ := url.Parse(urlString)

		r.URL.Path = ""
		r.URL.Host = url.Host
		r.URL.Scheme = url.Scheme
		r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
		r.Host = url.Host

		// Appears to append a trailing slash because of https://github.com/golang/go/pull/50339.
		// Not sure of workaround. Will comment as limitation and demonstrate with sites that support trailing slash
		httputil.NewSingleHostReverseProxy(url).ServeHTTP(w, r)

		_, err = conn.Exec(context.Background(), "INSERT INTO usage (short_link, occurred_at) VALUES ($1, $2)", id, time.Now())
		if err != nil {
			panic(err)
		}
	}
}

// lookup the long url for a given id/short_link
func lookup(conn *pgxpool.Pool, id string) (string, error) {
	var destination string

	err := conn.QueryRow(context.Background(), "SELECT destination FROM urls WHERE short_link=$1 AND (expiry > NOW() OR expiry IS NULL)", id).Scan(&destination)
	if err != nil {
		return "", err
	}

	return destination, nil
}

func main() {
	conn, err := pgxpool.Connect(context.Background(), "postgres://postgres:password@db:5432/url-shortener")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	router := mux.NewRouter()

	shortLinkRouter := router.PathPrefix("/v1/admin/short-link").Subrouter()
	shortLinkRouter.HandleFunc("", CreateRandomShortLink(conn)).Methods("POST")
	shortLinkRouter.HandleFunc("/{short_link}", CreateNamedShortLink(conn)).Methods("POST")
	shortLinkRouter.HandleFunc("/{short_link}", DeleteShortLink(conn)).Methods("DELETE")
	shortLinkRouter.HandleFunc("/{short_link}/analytics/7d", GetAnalyticsForShortLink(conn, "7 DAYS")).Methods("GET")
	shortLinkRouter.HandleFunc("/{short_link}/analytics/24h", GetAnalyticsForShortLink(conn, "24 HOURS")).Methods("GET")
	shortLinkRouter.HandleFunc("/{short_link}/analytics/all", GetAllAnalyticsForShortLink(conn)).Methods("GET")

	router.HandleFunc("/s/{short_link}", proxy(conn)).Methods("GET")

	// write unit tests and e2e script after docker-compose implemented. Maybe embed e2e script as extended part of docker-compose

	http.ListenAndServe(":80", router)
}
