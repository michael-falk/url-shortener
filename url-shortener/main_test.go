package main

import (
	"bytes"
	"context"
	"encoding/json"
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

func UnloadTestUsage(conn *pgxpool.Pool) {
	_, err := conn.Exec(context.Background(), "DELETE FROM urls WHERE tenant=$1", "integration_test")
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

func RequestNewRandomShortUrl(url string) string {
	jsonData, _ := json.Marshal(NewShortUrlRequest{Url: url})

	client := http.Client{}
	req, err := http.NewRequest("POST", "http://localhost:8080/v1/admin/short-urls", bytes.NewBuffer(jsonData))
	if err != nil {
		//Handle Error
	}

	req.Header = http.Header{
		"Content-Type": []string{"application/json"},
		"X-SUBJECT":    []string{"integration_test"},
	}

	resp, _ := client.Do(req)
	defer resp.Body.Close()

	var respBody NewShortUrlResponse
	body, _ := ioutil.ReadAll(resp.Body)
	json.Unmarshal(body, &respBody)

	return respBody.ShortUrl
}

func RequestNewNamedShortUrl(name, url string) string {
	jsonData, _ := json.Marshal(NewShortUrlRequest{Url: url})

	client := http.Client{}
	req, _ := http.NewRequest("POST", fmt.Sprintf("http://localhost:8080/v1/admin/short-urls/%s", name), bytes.NewBuffer(jsonData))

	req.Header = http.Header{
		"Content-Type": []string{"application/json"},
		"X-SUBJECT":    []string{"integration_test"},
	}

	resp, _ := client.Do(req)
	defer resp.Body.Close()

	var respBody NewShortUrlResponse
	body, _ := ioutil.ReadAll(resp.Body)
	json.Unmarshal(body, &respBody)

	return respBody.ShortUrl
}

func RequestListShortUrls() []string {
	client := http.Client{}
	req, _ := http.NewRequest("GET", "http://localhost:8080/v1/admin/short-urls", nil)

	req.Header = http.Header{
		"Content-Type": []string{"application/json"},
		"X-SUBJECT":    []string{"integration_test"},
	}

	resp, _ := client.Do(req)
	defer resp.Body.Close()

	var respBody ListShortUrlResponse
	body, _ := ioutil.ReadAll(resp.Body)
	json.Unmarshal(body, &respBody)

	return respBody.Urls
}

func RequestDeleteShortUrl(name string) {
	client := http.Client{}
	req, _ := http.NewRequest("DELETE", fmt.Sprintf("http://localhost:8080/v1/admin/short-urls/%s", name), nil)

	req.Header = http.Header{
		"Content-Type": []string{"application/json"},
		"X-SUBJECT":    []string{"integration_test"},
	}

	client.Do(req)
}

func TestCreateListAndDeleteRandom(t *testing.T) {
	url := RequestNewRandomShortUrl("https://www.cloudflare.com/")
	urls := RequestListShortUrls()
	assert.Contains(t, urls, url, "Expected URL to be listed")

	RequestDeleteShortUrl(url)
	urls = RequestListShortUrls()
	assert.NotContains(t, urls, url, "Expected URL to not be listed")
}

func TestCreateListAndDeleteNamed(t *testing.T) {
	name := RandomString(6)
	url := RequestNewNamedShortUrl(name, "https://www.cloudflare.com/")
	assert.Equal(t, name, url, "Expected chosen URL to be the returned URL")

	urls := RequestListShortUrls()
	assert.Contains(t, urls, url, "Expected URL to be listed")

	RequestDeleteShortUrl(url)
	urls = RequestListShortUrls()
	assert.NotContains(t, urls, url, "Expected URL to not be listed")
}

func TestRedirect(t *testing.T) {
	url := RequestNewRandomShortUrl("https://www.cloudflare.com/")

	resp1, _ := http.Get("https://www.cloudflare.com/")
	expected := resp1.Header.Get("link")

	resp2, _ := http.Get(fmt.Sprintf("http://localhost:8080/s/%s", url))
	actual := resp2.Header.Get("link")

	assert.Equal(t, expected, actual, "Expected redirect to match original link header")
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
	defer UnloadTestUsage(conn)

	code := m.Run()

	os.Exit(code)
}
