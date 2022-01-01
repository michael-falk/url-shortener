CREATE TABLE urls (
    short_url VARCHAR(16) PRIMARY KEY,
    tenant VARCHAR(32) NOT NULL,
    destination TEXT NOT NULL,
    expiry TIMESTAMP
);
CREATE TABLE usage (
    id SERIAL PRIMARY KEY,
    short_url VARCHAR(16) NOT NULL,
    occurred_at TIMESTAMP NOT NULL,
    CONSTRAINT fk_urls FOREIGN KEY(short_url) REFERENCES urls(short_url) ON DELETE CASCADE
);
