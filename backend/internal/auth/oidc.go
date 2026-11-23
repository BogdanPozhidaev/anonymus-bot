package auth

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/fastcheck/anonymus_bot/backend/internal/config"
	"github.com/redis/go-redis/v9"
	"golang.org/x/oauth2"
)

const (
	OIDCStateTTL  = 5 * time.Minute
	OIDCTicketTTL = time.Minute
)

var ErrOIDCAuthentication = errors.New("OIDC authentication failed")

type OIDCService struct {
	settings  config.OIDC
	client    *http.Client
	redis     *redis.Client
	discovery sync.Mutex
	oauth     *oauth2.Config
	verifier  *oidc.IDTokenVerifier
	bindings  map[int64]string
}

type oidcAttempt struct {
	Verifier string
	Nonce    string
}

type OIDCTicket struct {
	OperatorID  int64
	AuthVersion int64
	Binding     string
}

func NewOIDC(settings config.OIDC, client *http.Client, store *redis.Client) *OIDCService {
	o := &OIDCService{settings: settings, client: client, redis: store, bindings: make(map[int64]string)}
	for subject, id := range settings.Operators {
		o.bindings[id] = tokenKey("oidc_binding", settings.Issuer+"\x00"+settings.ClientID+"\x00"+subject)
	}
	return o
}

func (o *OIDCService) provider(ctx context.Context) (*oauth2.Config, *oidc.IDTokenVerifier, error) {
	o.discovery.Lock()
	defer o.discovery.Unlock()
	if o.oauth != nil {
		return o.oauth, o.verifier, nil
	}
	provider, err := oidc.NewProvider(oidc.ClientContext(ctx, o.client), o.settings.Issuer)
	if err != nil {
		return nil, nil, ErrOIDCAuthentication
	}
	var metadata struct {
		Keys        string   `json:"jwks_uri"`
		PKCEMethods []string `json:"code_challenge_methods_supported"`
	}
	endpoint := provider.Endpoint()
	if provider.Claims(&metadata) != nil || !secureProviderURL(endpoint.AuthURL) ||
		!secureProviderURL(endpoint.TokenURL) || !secureProviderURL(metadata.Keys) ||
		(len(metadata.PKCEMethods) > 0 && !slices.Contains(metadata.PKCEMethods, "S256")) {
		return nil, nil, ErrOIDCAuthentication
	}
	o.oauth = &oauth2.Config{ClientID: o.settings.ClientID, ClientSecret: o.settings.ClientSecret,
		RedirectURL: o.settings.RedirectURL, Endpoint: endpoint, Scopes: []string{oidc.ScopeOpenID}}
	o.verifier = provider.Verifier(&oidc.Config{ClientID: o.settings.ClientID})
	return o.oauth, o.verifier, nil
}

func secureProviderURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Fragment == ""
}

func (o *OIDCService) Begin(ctx context.Context) (authorizationURL, browser string, err error) {
	oauth, _, err := o.provider(ctx)
	if err != nil {
		return "", "", err
	}
	state, browser := oauth2.GenerateVerifier(), oauth2.GenerateVerifier()
	attempt := oidcAttempt{Verifier: oauth2.GenerateVerifier(), Nonce: oauth2.GenerateVerifier()}
	if err := o.store(ctx, tokenKey("oidc_state", state+"."+browser), attempt, OIDCStateTTL); err != nil {
		return "", "", err
	}
	return oauth.AuthCodeURL(state, oauth2.S256ChallengeOption(attempt.Verifier), oidc.Nonce(attempt.Nonce)), browser, nil
}

func (o *OIDCService) Verify(ctx context.Context, state, browser, code string) (int64, error) {
	if !validOIDCHandle(state) || !validOIDCHandle(browser) || len(code) == 0 || len(code) > 8192 {
		return 0, ErrOIDCAuthentication
	}
	var attempt oidcAttempt
	if err := o.consume(ctx, tokenKey("oidc_state", state+"."+browser), &attempt); err != nil {
		return 0, err
	}
	oauth, verifier, err := o.provider(ctx)
	if err != nil {
		return 0, err
	}
	tokens, err := oauth.Exchange(oidc.ClientContext(ctx, o.client), code, oauth2.VerifierOption(attempt.Verifier))
	if err != nil {
		return 0, ErrOIDCAuthentication
	}
	raw, ok := tokens.Extra("id_token").(string)
	if !ok || len(raw) > 64<<10 {
		return 0, ErrOIDCAuthentication
	}
	token, err := verifier.Verify(ctx, raw)
	if err != nil || token.Issuer != o.settings.Issuer || token.Subject == "" ||
		subtle.ConstantTimeCompare([]byte(token.Nonce), []byte(attempt.Nonce)) != 1 {
		return 0, ErrOIDCAuthentication
	}
	var claims struct {
		AuthorizedParty string `json:"azp"`
	}
	if token.Claims(&claims) != nil ||
		((len(token.Audience) > 1 || claims.AuthorizedParty != "") && claims.AuthorizedParty != o.settings.ClientID) {
		return 0, ErrOIDCAuthentication
	}
	id := o.settings.Operators[token.Subject]
	if id == 0 {
		return 0, ErrOIDCAuthentication
	}
	return id, nil
}

func (o *OIDCService) CreateTicket(ctx context.Context, id, version int64) (string, error) {
	ticket := oauth2.GenerateVerifier()
	return ticket, o.store(ctx, tokenKey("oidc_ticket", ticket), OIDCTicket{
		OperatorID: id, AuthVersion: version, Binding: o.bindings[id],
	}, OIDCTicketTTL)
}

func (o *OIDCService) UseTicket(ctx context.Context, ticket string) (OIDCTicket, error) {
	var value OIDCTicket
	if !validOIDCHandle(ticket) {
		return value, ErrOIDCAuthentication
	}
	if err := o.consume(ctx, tokenKey("oidc_ticket", ticket), &value); err != nil {
		return value, err
	}
	if value.Binding == "" || o.bindings[value.OperatorID] != value.Binding {
		return value, ErrOIDCAuthentication
	}
	return value, nil
}

func (o *OIDCService) store(ctx context.Context, key string, data any, ttl time.Duration) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return o.redis.Set(ctx, key, payload, ttl).Err()
}

func (o *OIDCService) consume(ctx context.Context, key string, data any) error {
	payload, err := o.redis.GetDel(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return ErrOIDCAuthentication
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, data)
}

func validOIDCHandle(handle string) bool {
	if len(handle) != 43 {
		return false
	}
	_, err := base64.RawURLEncoding.Strict().DecodeString(handle)
	return err == nil
}
