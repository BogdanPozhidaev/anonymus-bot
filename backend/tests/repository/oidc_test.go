package repository_test

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fastcheck/anonymus_bot/backend/internal/api"
	"github.com/fastcheck/anonymus_bot/backend/internal/auth"
	"github.com/fastcheck/anonymus_bot/backend/internal/config"
	appcrypto "github.com/fastcheck/anonymus_bot/backend/internal/crypto"
	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
)

func TestOIDCWithSignedProviderAndLocalTOTP(t *testing.T) {
	ctx := t.Context()
	pool := setupTestDB(t)
	store := setupThrottleRedis(t)
	operators := repository.NewOperatorRepository(pool)
	operator := models.Operator{Name: "Fixture", Login: "oidc-fixture", Role: "operator", Status: "active", Language: "en"}
	if err := operators.Create(ctx, &operator); err != nil {
		t.Fatal(err)
	}
	enc, err := appcrypto.NewEncryptor(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	pending := auth.NewPendingAuthService(store, enc)
	const localSecret = "JBSWY3DPEHPK3PXP"
	stored, err := pending.EncodeTOTP(localSecret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE operators SET totp_secret=$1 WHERE id=$2", stored, operator.ID); err != nil {
		t.Fatal(err)
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	encode := base64.RawURLEncoding.EncodeToString
	type exchange struct{ challenge, token string }
	var issued atomic.Pointer[exchange]
	var exchanges atomic.Int32
	const clientID, clientSecret, subject = "fixture-client", "fixture-secret", "fixture-subject"
	const redirectURL = "https://panel.example.invalid" + config.OIDCCallbackPath
	var issuer string
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var response any
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			response = map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/authorize",
				"token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys",
				"code_challenge_methods_supported": []string{"S256"}, "id_token_signing_alg_values_supported": []string{"RS256"}}
		case "/keys":
			response = map[string]any{"keys": []map[string]string{{"kty": "RSA", "alg": "RS256", "use": "sig", "kid": "fixture",
				"n": encode(key.N.Bytes()), "e": encode(big.NewInt(int64(key.E)).Bytes())}}}
		case "/token":
			exchanges.Add(1)
			x := issued.Load()
			user, password, ok := r.BasicAuth()
			hash := sha256.Sum256([]byte(r.FormValue("code_verifier")))
			if x == nil || !ok || user != clientID || password != clientSecret || encode(hash[:]) != x.challenge ||
				r.FormValue("redirect_uri") != redirectURL || r.FormValue("code") != "fixture-code" {
				t.Error("token exchange did not use the client credentials and bound verifier")
				http.Error(w, "invalid grant", http.StatusBadRequest)
				return
			}
			response = map[string]any{"access_token": "fixture-access-token", "token_type": "Bearer", "id_token": x.token}
		default:
			http.NotFound(w, r)
			return
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Error(err)
		}
	}))
	defer provider.Close()
	issuer = provider.URL
	sessions := auth.NewSessionService(store)
	server := api.NewServer(pool, store, slog.New(slog.NewTextHandler(io.Discard, nil)), nil,
		repository.NewSessionRepository(pool), nil, operators, nil, repository.NewAuditLogRepository(pool),
		nil, sessions, pending, nil, "", "")
	router := gin.New()
	server.EnableOIDC(auth.NewOIDC(config.OIDC{Issuer: issuer, ClientID: clientID, ClientSecret: clientSecret,
		RedirectURL: redirectURL, Operators: map[string]int64{subject: operator.ID}}, provider.Client(), store))
	server.RegisterRoutes(router, strings.Repeat("a", 64))
	var requestNumber int
	request := func(method, path, payload, token string, cookie *http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		requestNumber++
		req := httptest.NewRequest(method, path, strings.NewReader(payload))
		req.RemoteAddr = fmt.Sprintf("192.0.2.%d:9876", requestNumber)
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	findCookie := func(response *httptest.ResponseRecorder, suffix string) *http.Cookie {
		t.Helper()
		for _, cookie := range response.Result().Cookies() {
			if strings.HasSuffix(cookie.Name, suffix) && cookie.Value != "" {
				if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" {
					t.Fatal("unsafe OIDC cookie")
				}
				return cookie
			}
		}
		t.Fatal("OIDC cookie missing")
		return nil
	}
	begin := func(change func(map[string]any)) (string, *http.Cookie) {
		t.Helper()
		response := request("POST", "/api/auth/oidc/start", "", "", nil)
		var start struct {
			URL string `json:"authorization_url"`
		}
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &start) != nil {
			t.Fatalf("start returned %d: %s", response.Code, response.Body)
		}
		location, err := url.Parse(start.URL)
		if err != nil {
			t.Fatal(err)
		}
		query := location.Query()
		if query.Get("nonce") == "" || query.Get("state") == "" || query.Get("code_challenge_method") != "S256" ||
			query.Get("code_challenge") == "" || query.Get("scope") != "openid" {
			t.Fatal("authorization request omitted OIDC or PKCE state")
		}
		claims := map[string]any{"iss": issuer, "sub": subject, "aud": clientID, "nonce": query.Get("nonce"),
			"iat": time.Now().Add(-time.Minute).Unix(), "exp": time.Now().Add(time.Minute).Unix(),
			"role": "admin", "email": "untrusted@example.invalid"}
		if change != nil {
			change(claims)
		}
		payload, err := json.Marshal(claims)
		if err != nil {
			t.Fatal(err)
		}
		content := encode([]byte(`{"alg":"RS256","kid":"fixture"}`)) + "." + encode(payload)
		digest := sha256.Sum256([]byte(content))
		signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
		if err != nil {
			t.Fatal(err)
		}
		if claims["invalid_signature"] == true {
			signature[0] ^= 1
		}
		issued.Store(&exchange{challenge: query.Get("code_challenge"), token: content + "." + encode(signature)})
		callback := config.OIDCCallbackPath + "?code=fixture-code&state=" + url.QueryEscape(query.Get("state"))
		return callback, findCookie(response, "-browser")
	}
	assertCallback := func(path string, cookie *http.Cookie, want string) *httptest.ResponseRecorder {
		t.Helper()
		response := request("GET", path, "", "", cookie)
		if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/login?oidc="+want ||
			response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("unexpected callback %d %s", response.Code, response.Header().Get("Location"))
		}
		return response
	}
	for _, tc := range []struct {
		label, claim string
		value        any
	}{
		{"wrong nonce", "nonce", "wrong"}, {"wrong issuer", "iss", "https://other.example.invalid"},
		{"wrong audience", "aud", "other"}, {"wrong authorized party", "azp", "other"},
		{"expired assertion", "exp", int64(1)}, {"invalid signature", "invalid_signature", true},
		{"unlinked identity", "sub", "unlinked"},
	} {
		t.Run(tc.label, func(t *testing.T) {
			path, browser := begin(func(claims map[string]any) { claims[tc.claim] = tc.value })
			assertCallback(path, browser, "failed")
		})
	}
	for _, statement := range []string{
		"UPDATE operators SET status='disabled' WHERE id=$1",
		"UPDATE operators SET status='active', must_change_password=true WHERE id=$1",
		"UPDATE operators SET must_change_password=false,totp_secret=NULL WHERE id=$1",
	} {
		if _, err := pool.Exec(ctx, statement, operator.ID); err != nil {
			t.Fatal(err)
		}
		path, browser := begin(nil)
		assertCallback(path, browser, "failed")
	}
	if _, err := pool.Exec(ctx, "UPDATE operators SET totp_secret=$1 WHERE id=$2", stored, operator.ID); err != nil {
		t.Fatal(err)
	}
	stalePath, staleBrowser := begin(nil)
	staleTicket := findCookie(assertCallback(stalePath, staleBrowser, "complete"), "-ticket")
	if _, err := pool.Exec(ctx, "UPDATE operators SET auth_version=auth_version+1 WHERE id=$1", operator.ID); err != nil {
		t.Fatal(err)
	}
	if request("POST", "/api/auth/oidc/complete", "", "", staleTicket).Code != http.StatusUnauthorized {
		t.Fatal("an authentication version change did not invalidate the handoff")
	}
	path, browser := begin(nil)
	before := exchanges.Load()
	assertCallback(path, nil, "failed")
	assertCallback(path+"&state=duplicate", browser, "failed")
	if exchanges.Load() != before {
		t.Fatal("callback without the browser binding reached the provider")
	}
	ticket := findCookie(assertCallback(path, browser, "complete"), "-ticket")
	assertCallback(path, browser, "failed")
	completed := request("POST", "/api/auth/oidc/complete", "", "", ticket)
	var challenge struct {
		Status  string
		Pending string `json:"pending_token"`
	}
	if completed.Code != http.StatusOK || json.Unmarshal(completed.Body.Bytes(), &challenge) != nil ||
		challenge.Status != "totp_required" || challenge.Pending == "" || strings.Contains(completed.Body.String(), "session_token") {
		t.Fatalf("SSO bypassed local TOTP or failed: %d", completed.Code)
	}
	if request("POST", "/api/auth/oidc/complete", "", "", ticket).Code != http.StatusUnauthorized ||
		request("GET", "/api/sessions", "", challenge.Pending, nil).Code != http.StatusUnauthorized {
		t.Fatal("handoff replay or pending token authenticated")
	}
	code, err := totp.GenerateCode(localSecret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	response := request("POST", "/api/auth/totp/verify", fmt.Sprintf(`{"pending_token":%q,"code":%q}`, challenge.Pending, code), "", nil)
	var signedIn struct {
		Token string `json:"session_token"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &signedIn) != nil {
		t.Fatalf("local TOTP verification returned %d", response.Code)
	}
	session, err := sessions.Get(ctx, signedIn.Token)
	if err != nil || session.OperatorID != operator.ID || session.Role != "operator" {
		t.Fatalf("SSO did not preserve the local role: %v", err)
	}
}
