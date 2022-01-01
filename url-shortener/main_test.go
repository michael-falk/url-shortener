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
	mockOccurredAt := time.Now().Add(-time.Hour)

	for i := 0; i < 8; i++ {
		conn.Exec(context.Background(), "INSERT INTO usage (short_url, occurred_at) VALUES ($1, $2)", id, mockOccurredAt)
		mockOccurredAt = time.Now().Add(time.Hour * -24)
	}
}

func UnloadTestUsage(conn *pgxpool.Pool, id string) {
	conn.Exec(context.Background(), "DELETE FROM usage WHERE short_url=$1", id)
}

func TestAnalyticsAll(t *testing.T) {
	resp, _ := http.Get(fmt.Sprintf("http://localhost:8080/v1/admin/short-urls/%s/analytics/all", analyticsShortUrl))
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	assert.Equal(t, 200, resp.StatusCode, "OK response is expected")
	assert.Equal(t, fmt.Sprintf("%s has been called 8 times for all time", analyticsShortUrl), body, "7 uses expected")
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
}
