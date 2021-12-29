CREATE TABLE urls (
    short_link VARCHAR(16) PRIMARY KEY,
    tenant VARCHAR(32) NOT NULL,
    destination TEXT NOT NULL,
    expiry TIMESTAMP
);
CREATE TABLE usage (
    id SERIAL PRIMARY KEY,
    short_link VARCHAR(16) NOT NULL,
    occurred_at TIMESTAMP NOT NULL,
    CONSTRAINT fk_urls FOREIGN KEY(short_link) REFERENCES urls(short_link) ON DELETE CASCADE
);
