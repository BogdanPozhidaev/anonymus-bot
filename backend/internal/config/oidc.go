package config

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"
	"unicode"
)

const OIDCCallbackPath = "/api/auth/oidc/callback"

type OIDC struct {
	Issuer, ClientID, ClientSecret, RedirectURL string
	Operators map[string]int64
}

func httpsURL(value string) (*url.URL, bool) {
	u, err := url.Parse(value)
	return u, err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil &&
		!strings.ContainsAny(value, "?#")
}

func loadOIDC() (*OIDC, error) {
	issuer := os.Getenv("OIDC_ISSUER_URL")
	if issuer == "" {
		return nil, nil
	}
	if _, ok := httpsURL(issuer); !ok {
		return nil, errors.New("OIDC_ISSUER_URL must be an HTTPS issuer URL without credentials, query or fragment")
	}
	redirect := os.Getenv("OIDC_REDIRECT_URL")
	u, ok := httpsURL(redirect)
	if !ok || u.EscapedPath() != OIDCCallbackPath {
		return nil, errors.New("OIDC_REDIRECT_URL must be the HTTPS origin followed by /api/auth/oidc/callback")
	}
	id := os.Getenv("OIDC_CLIENT_ID")
	if id == "" || len(id) > 255 || strings.ContainsFunc(id, unicode.IsSpace) {
		return nil, errors.New("OIDC_CLIENT_ID is invalid")
	}
	secret, err := os.ReadFile(os.Getenv("OIDC_CLIENT_SECRET_FILE"))
	if err != nil || len(secret) > 4096 || strings.TrimSpace(string(secret)) == "" {
		return nil, errors.New("OIDC_CLIENT_SECRET_FILE must contain the client secret")
	}
	data, err := os.ReadFile(os.Getenv("OIDC_OPERATOR_MAP_FILE"))
	if err != nil {
		return nil, errors.New("cannot read OIDC_OPERATOR_MAP_FILE")
	}
	var links []struct {
		Subject string `json:"subject"`
		OperatorID int64 `json:"operator_id"`
	}
	if len(data) > 1<<20 || json.Unmarshal(data, &links) != nil || len(links) == 0 {
		return nil, errors.New("OIDC_OPERATOR_MAP_FILE must contain a JSON array of subject and operator_id mappings")
	}
	operators := make(map[string]int64, len(links))
	ids := make(map[int64]bool, len(links))
	for _, link := range links {
		if link.Subject == "" || len(link.Subject) > 255 || strings.TrimSpace(link.Subject) != link.Subject ||
			strings.ContainsFunc(link.Subject, unicode.IsControl) || link.OperatorID <= 0 ||
			operators[link.Subject] != 0 || ids[link.OperatorID] {
			return nil, errors.New("OIDC_OPERATOR_MAP_FILE must contain unique, valid subjects and operator IDs")
		}
		operators[link.Subject] = link.OperatorID
		ids[link.OperatorID] = true
	}
	return &OIDC{Issuer: issuer, ClientID: id, ClientSecret: strings.TrimSpace(string(secret)),
		RedirectURL: redirect, Operators: operators}, nil
}
