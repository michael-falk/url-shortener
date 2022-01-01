package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/stretchr/testify/assert"
)

func LoadTestUsage(conn *pgxpool.Pool, id string) {
	mockOccurredAt := time.Now().Add(time.Hour * -12)

	_, err := conn.Exec(context.Background(), "INSERT INTO urls (short_url, tenant, destination) VALUES ($1, $2, $3)", id, "integration_test", "http://cloudflare.com")
	if err != nil {
		panic(err)
	}

	for i := 0; i < 8; i++ {
		_, err = conn.Exec(context.Background(), "INSERT INTO usage (short_url, occurred_at) VALUES ($1, $2)", id, mockOccurredAt)
		if err != nil {
			panic(err)
		}

		mockOccurredAt = mockOccurredAt.Add(time.Hour * -24)
	}
}

func UnloadTestUsage(conn *pgxpool.Pool, id string) {
	_, err := conn.Exec(context.Background(), "DELETE FROM usage WHERE short_url=$1", id)
	if err != nil {
		panic(err)
	}
}

func TestAnalyticsAll(t *testing.T) {
	resp, _ := http.Get(fmt.Sprintf("http://localhost:8080/v1/admin/short-urls/%s/analytics/all", analyticsShortUrl))
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	assert.Equal(t, 200, resp.StatusCode, "OK response is expected")
	assert.Equal(t, fmt.Sprintf("%s has been called 8 times for all time\n", analyticsShortUrl), string(body), "8 uses expected")
}

func TestAnalytics7d(t *testing.T) {
	resp, _ := http.Get(fmt.Sprintf("http://localhost:8080/v1/admin/short-urls/%s/analytics/7d", analyticsShortUrl))
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	assert.Equal(t, 200, resp.StatusCode, "OK response is expected")
	assert.Equal(t, fmt.Sprintf("%s has been called 7 times in 7 DAYS\n", analyticsShortUrl), string(body), "7 uses expected")
}

func TestAnalytics24h(t *testing.T) {
	resp, _ := http.Get(fmt.Sprintf("http://localhost:8080/v1/admin/short-urls/%s/analytics/24h", analyticsShortUrl))
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	assert.Equal(t, 200, resp.StatusCode, "OK response is expected")
	assert.Equal(t, fmt.Sprintf("%s has been called 1 times in 24 HOURS\n", analyticsShortUrl), string(body), "1 use expected")
}

var analyticsShortUrl string

func TestMain(m *testing.M) {
	conn, err := pgxpool.Connect(context.Background(), "postgres://postgres:password@localhost:5432/url-shortener")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	analyticsShortUrl = RandomString(6)
	LoadTestUsage(conn, analyticsShortUrl)
	defer UnloadTestUsage(conn, analyticsShortUrl)

	code := m.Run()

	os.Exit(code)
}
