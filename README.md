# url-shortener
Proof-of-concept url shortener (to learn golang).

## Design
This design required support for the following:

> A short URL:
> - Has one long URL
> - This URL shortener should have a well-defined API for URLs created, including analytics of usage.
> - No duplicate URLs are allowed to be created.
> - Short links can expire at a future time or can live forever.

I made the following assumptions:
- Users will supply valid destination URLs.
  - Furthermore, because of a [known bug](https://github.com/golang/go/pull/50339) in go's built-in reverse proxy library around trailing slashes, destination urls will support strict trailing slashes.
- Users in a production environment would use some auth mechanism (e.g. OAuth2) where claims can be used to determine access-control, etc. For simplicity, clients will explicitly set an `X-SUBJECT` header which would be analogous to a `JWT` `sub` claim.
- Clients will not be able to modify a short-url in-place. Rather they will have to delete and recreate a URL.
- The analytics endpoint will be strongly coupled to a short-url. If a link is deleted, so will the corresponding analytics data
- The analytics endpoint will strictly support -7d, -24h, and all time. Nothing more complex or flexible.
- An expired URL is not automatically deleted, rather disabled. An admin must delete the URL to free it.

### Persistence
To support other requirements around persistence and the expected queries, I chose to use PostgreSQL. I have 1 table as a KV store (a mapping from short_url to long_url) and another as append-log (for analytic queries around usage). I felt a single SQL store would satisfy all use cases rather than Mongo or ephemeral Redis.

### Web API
The Web API should be a straight-forward CRUD API to create a short-url and a shorter 'data-plane' API for the actual short-url.
The routes are as follow:
```
# Management APIs
# Requires a X-SUBJECT header for the owning tenant
GET /v1/admin/short-url
POST /v1/admin/short-url
POST /v1/admin/short-url/:short_url # POST chosen because write is not idempotent. Updates are not supported
DELETE /v1/admin/short-url/:short_url

# Analytics APIs
GET /v1/admin/short-url/:short_url/analytics/24h
GET /v1/admin/short-url/:short_url/analytics/7d
GET /v1/admin/short-url/:short_url/analytics/all

# Short URL Redirection
GET /s/:short_url
```

The write endpoints have the following request body:
```json
{
    "url": "some http endpoint",
    "expiry": "Optional: Some ISO-8601 datetime after which the short-url is disabled" 
}
```

A developer should be able to to enumerate all short_urls that they own, and from their manage their urls (i.e. Create, Delete, etc.).

Some examples cURL requests below:
```bash
# example redirect to pre-loaded short url
curl -H X-SUBJECT:e2e-test localhost:8080/s/test1

# Example list all short_urls for the `e2e-test` tenant
curl -H X-SUBJECT:e2e-test localhost:8080/v1/admin/short-urls | jq

# Example auto-generated short url
curl -H X-SUBJECT:e2e-test -X POST localhost:8080/v1/admin/short-urls -d '{ "url": "http://foo.bar" }' | jq

# Example create a named short url with some expiry
curl -H X-SUBJECT:e2e-test -X POST localhost:8080/v1/admin/short-urls/named -d '{ "url": "http://foo.bar", "expireAt": "2021-02-18T21:54:42.123Z" }' | jq

# Example delete a named short url
curl -H X-SUBJECT:e2e-test -X DELETE localhost:8080/v1/admin/short-urls/named

# Example analytics queries
curl http://localhost:8080/v1/admin/short-urls/test1/analytics/24h # Past 24 hours
curl http://localhost:8080/v1/admin/short-urls/test1/analytics/7d  # Past 7 days
curl http://localhost:8080/v1/admin/short-urls/test1/analytics/all # For all time
```

## Operating Instructions
This demo assumes you have docker installed locally and can run `docker-compose`.

### Build
To (re-)build this demo, run the following:
```sh
docker-compose build
```

### Run service locally
To run locally in the background:
```sh
docker-compose up -d
```

To run with integrated logging for all services:
```sh
docker-compose up
```
