package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strings"
)

// NormalizeURL canonicalizes a URL so trivially-different forms of the same page
// collapse to one corpus entry: lowercase scheme/host, drop fragments, strip
// common tracking params, and remove trailing slashes.
func NormalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return strings.ToLower(strings.TrimRight(raw, "/"))
	}

	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Host = strings.TrimPrefix(u.Host, "www.")
	u.Fragment = ""

	if u.RawQuery != "" {
		q := u.Query()
		for k := range q {
			lk := strings.ToLower(k)
			if strings.HasPrefix(lk, "utm_") || lk == "ref" || lk == "fbclid" || lk == "gclid" {
				q.Del(k)
			}
		}
		u.RawQuery = q.Encode()
	}

	u.Path = strings.TrimRight(u.Path, "/")
	return u.String()
}

// HashURL returns a stable key for a URL after normalization.
func HashURL(raw string) string {
	return sha256hex(NormalizeURL(raw))
}

// Domain extracts the registrable host (sans "www.") from a URL, "" on failure.
func DomainOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Host), "www.")
}

// NormalizeQuery canonicalizes a search query for cache keying: lowercase,
// collapse internal whitespace, and trim.
func NormalizeQuery(q string) string {
	return strings.Join(strings.Fields(strings.ToLower(q)), " ")
}

// HashQuery returns a stable key for a normalized search query.
func HashQuery(q string) string {
	return sha256hex(NormalizeQuery(q))
}

func sha256hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
