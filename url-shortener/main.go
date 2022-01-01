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

type NewShortUrlRequest struct {
	Url      string     `json:"url"`
	ExpireAt *time.Time `json:"expireAt,omitempty"` // RFC3339 datetime
}

type NewShortUrlResponse struct {
	ShortUrl string `json:"shortUrl"`
}

type ListShortUrlResponse struct {
	Urls []string `json:"urls"`
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

func ListShortUrls(conn *pgxpool.Pool) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant := r.Header.Get("X-SUBJECT")
		urls := make([]string, 0)

		rows, _ := conn.Query(context.Background(), "SELECT short_url FROM urls WHERE tenant=$1", tenant)

		var url string
		for rows.Next() {

			rows.Scan(&url)
			urls = append(urls, url)
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		resp := ListShortUrlResponse{
			Urls: urls,
		}
		jsonResp, _ := json.Marshal(resp)
		w.Write(jsonResp)
	}
}

// TODO auth so that owning engineering tenant will have permissions to call the WRITE admin route for the short link. Using JWT semantics. Will not verify tokens. just check `sub` to identify tenant for short-urls.
// For now, cheat and make a custom X-SUBJECT header to pass in tenant information, etc.

func CreateNamedShortUrl(conn *pgxpool.Pool) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["short_url"]
		tenant := r.Header.Get("X-SUBJECT")

		var req NewShortUrlRequest
		json.NewDecoder(r.Body).Decode(&req)

		HandleNewShortUrlRequest(conn, tenant, id, req, w)
	}
}

func CreateRandomShortUrl(conn *pgxpool.Pool) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var req NewShortUrlRequest
		json.NewDecoder(r.Body).Decode(&req)
		tenant := r.Header.Get("X-SUBJECT")

		id := RandomString(6) // TODO make configurable

		HandleNewShortUrlRequest(conn, tenant, id, req, w)
	}
}

func HandleNewShortUrlRequest(conn *pgxpool.Pool, tenant, id string, req NewShortUrlRequest, w http.ResponseWriter) {
	if tenant == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Missing required header: X-SUBJECT")
		return
	}

	if req.Url == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Missing request body field: url")
		return
	}

	_, err := url.Parse(req.Url)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Invalid destination URL")
		return
	}

	if req.ExpireAt == nil {
		_, err = conn.Exec(context.Background(), "INSERT INTO urls (short_url, tenant, destination) VALUES ($1, $2, $3)", id, tenant, req.Url)
	} else {
		_, err = conn.Exec(context.Background(), "INSERT INTO urls (short_url, tenant, destination, expiry) VALUES ($1, $2, $3, $4)", id, tenant, req.Url, req.ExpireAt)
	}

	// err will be from a key-collision so respond with 409.
	if err != nil {
		w.WriteHeader(http.StatusConflict)
		fmt.Fprintf(w, "Short-URL already reserved: %s\n", id)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")

	resp := NewShortUrlResponse{
		ShortUrl: id,
	}

	jsonResp, _ := json.Marshal(resp)
	w.Write(jsonResp)
}

func DeleteShortUrl(conn *pgxpool.Pool) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["short_url"]
		tenant := r.Header.Get("X-SUBJECT")

		_, err := conn.Exec(context.Background(), "DELETE FROM urls WHERE short_url=$1 AND tenant=$2", id, tenant)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func GetAllAnalyticsForShortUrl(conn *pgxpool.Pool) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["short_url"]
		var count int64
		err := conn.QueryRow(context.Background(), "SELECT COUNT(id) FROM usage WHERE short_url=$1", id).Scan(&count)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
		}

		fmt.Fprintf(w, "%s has been called %d times for all time\n", id, count)

	}
}

func GetAnalyticsForShortUrl(conn *pgxpool.Pool, interval string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["short_url"]
		var count int64
		err := conn.QueryRow(context.Background(), "SELECT COUNT(id) FROM usage WHERE short_url=$1"+fmt.Sprintf("AND occurred_at > NOW() - INTERVAL '%s'", interval), id).Scan(&count)
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
		id := vars["short_url"]

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

		_, err = conn.Exec(context.Background(), "INSERT INTO usage (short_url, occurred_at) VALUES ($1, $2)", id, time.Now())
		if err != nil {
			panic(err)
		}
	}
}

// lookup the long url for a given id/short_url
func lookup(conn *pgxpool.Pool, id string) (string, error) {
	var destination string

	err := conn.QueryRow(context.Background(), "SELECT destination FROM urls WHERE short_url=$1 AND (expiry > NOW() OR expiry IS NULL)", id).Scan(&destination)
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

	shortUrlRouter := router.PathPrefix("/v1/admin/short-urls").Subrouter()
	shortUrlRouter.HandleFunc("", ListShortUrls(conn)).Methods("GEt")
	shortUrlRouter.HandleFunc("", CreateRandomShortUrl(conn)).Methods("POST")
	shortUrlRouter.HandleFunc("/{short_url}", CreateNamedShortUrl(conn)).Methods("POST")
	shortUrlRouter.HandleFunc("/{short_url}", DeleteShortUrl(conn)).Methods("DELETE")
	shortUrlRouter.HandleFunc("/{short_url}/analytics/7d", GetAnalyticsForShortUrl(conn, "7 DAYS")).Methods("GET")
	shortUrlRouter.HandleFunc("/{short_url}/analytics/24h", GetAnalyticsForShortUrl(conn, "24 HOURS")).Methods("GET")
	shortUrlRouter.HandleFunc("/{short_url}/analytics/all", GetAllAnalyticsForShortUrl(conn)).Methods("GET")

	router.HandleFunc("/s/{short_url}", proxy(conn)).Methods("GET")

	// write unit tests and e2e script after docker-compose implemented. Maybe embed e2e script as extended part of docker-compose

	http.ListenAndServe(":80", router)
}
