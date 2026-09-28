// Package portaltest serves a stateful double of the Gruntwork Developer Portal
// for tests: the device authorization grant behind `terragrunt login`, the
// verification page the user approves it on, and the catalog API the issued
// credential unlocks.
//
// Serve it with [net/http/httptest.NewTestServer] inside a [testing/synctest]
// bubble. The in-memory network carries no traffic off the process, and the
// bubble's clock lets a test wait out a poll interval or an expiry instantly.
package portaltest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/vhttp"
)

const (
	AuthorizePath    = "/api/v1/oauth/device/authorize"
	TokenPath        = "/api/v1/oauth/device/token"
	CatalogPath      = "/api/v1/catalog"
	VerificationPath = "/auth/device"

	ClientID          = "terragrunt-cli"
	ScopeCatalogRead  = "catalog:read"
	DeviceCodeGrant   = "urn:ietf:params:oauth:grant-type:device_code"
	UserCodeParam     = "user_code"
	DecisionParam     = "decision"
	TokenTypeBearer   = "Bearer"
	formContentType   = "application/x-www-form-urlencoded"
	jsonContentType   = "application/json"
	htmlContentType   = "text/html; charset=utf-8"
	bearerPrefix      = TokenTypeBearer + " "
	slowDownIncrement = 5 * time.Second
)

const (
	ErrorAuthorizationPending = "authorization_pending"
	ErrorSlowDown             = "slow_down"
	ErrorAccessDenied         = "access_denied"
	ErrorExpiredToken         = "expired_token"
	ErrorInvalidGrant         = "invalid_grant"
	ErrorInvalidClient        = "invalid_client"
	ErrorInvalidScope         = "invalid_scope"
	ErrorInvalidRequest       = "invalid_request"
	ErrorInvalidToken         = "invalid_token"
)

const (
	DefaultInterval      = 5 * time.Second
	DefaultExpiresIn     = 10 * time.Minute
	DefaultTokenLifetime = 30 * 24 * time.Hour
)

// Decision is the answer the user gives on the verification page.
type Decision string

const (
	Approve Decision = "approve"
	Deny    Decision = "deny"
)

// ErrVerificationRefused reports a verification page that would not take the
// user's decision.
var ErrVerificationRefused = errors.New("the verification page refused the decision")

// Config shapes what the portal answers. A zero duration takes its default.
type Config struct {
	// OrgName is the org name the issued token carries.
	OrgName string

	// AccountEmail is the email of the account the issued token names as the
	// one that approved the login.
	AccountEmail string

	// Repositories are the URLs the catalog API serves to an issued token.
	Repositories []string

	// Interval is the poll interval the device authorization names.
	Interval time.Duration

	// ExpiresIn is how long a login request waits for the user's decision.
	ExpiresIn time.Duration

	// TokenLifetime is how long an issued token stays good.
	TokenLifetime time.Duration

	// OrgID is the org the issued token is scoped to. Zero means 1.
	OrgID int64

	// SlowDowns is how many polls the portal throttles with slow_down before it
	// answers any of them, whatever their pace.
	SlowDowns int
}

// Stats counts what the portal has been asked since it started.
type Stats struct {
	// Violations names each request that broke the wire contract.
	Violations []string

	// Authorizations counts login requests the portal created.
	Authorizations int

	// PendingPolls counts polls answered with authorization_pending.
	PendingPolls int

	// SlowDowns counts polls answered with slow_down.
	SlowDowns int

	// TooFastPolls counts the slow_down polls that arrived before the interval
	// the client was told to keep.
	TooFastPolls int

	// TokensIssued counts credentials the token endpoint handed out.
	TokensIssued int

	// CatalogServed counts catalog requests that carried a good credential.
	CatalogServed int

	// CatalogRejected counts catalog requests whose credential was missing,
	// unknown, expired, or revoked.
	CatalogRejected int
}

// Portal is the double, served as an [http.Handler].
type Portal struct {
	t        testing.TB
	requests map[string]*loginRequest
	byUser   map[string]*loginRequest
	tokens   map[string]*issuedToken
	cfg      Config
	stats    Stats
	mu       sync.Mutex
}

type loginState int

const (
	statePending loginState = iota
	stateApproved
	stateDenied
	stateRedeemed
)

type loginRequest struct {
	lastPoll  time.Time
	expiresAt time.Time
	interval  time.Duration
	state     loginState
}

type issuedToken struct {
	expiresAt time.Time
	revoked   bool
}

// New returns a portal answering as config describes. A response it fails to
// write is logged to tb rather than failed.
func New(tb testing.TB, config *Config) *Portal {
	tb.Helper()

	cfg := *config

	if cfg.Interval == 0 {
		cfg.Interval = DefaultInterval
	}

	if cfg.ExpiresIn == 0 {
		cfg.ExpiresIn = DefaultExpiresIn
	}

	if cfg.TokenLifetime == 0 {
		cfg.TokenLifetime = DefaultTokenLifetime
	}

	if cfg.OrgID == 0 {
		cfg.OrgID = 1
	}

	return &Portal{
		t:        tb,
		cfg:      cfg,
		requests: map[string]*loginRequest{},
		byUser:   map[string]*loginRequest{},
		tokens:   map[string]*issuedToken{},
	}
}

// ServeHTTP answers one request as the portal would.
func (p *Portal) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch r.URL.Path {
	case AuthorizePath:
		p.authorize(w, r)
	case TokenPath:
		p.token(w, r)
	case VerificationPath:
		p.verify(w, r)
	case CatalogPath:
		p.catalog(w, r)
	default:
		p.violatef("request to unknown path %s", r.URL.Path)
		http.NotFound(w, r)
	}
}

// Stats returns a snapshot of what the portal has been asked.
func (p *Portal) Stats() Stats {
	p.mu.Lock()
	defer p.mu.Unlock()

	stats := p.stats
	stats.Violations = append([]string(nil), p.stats.Violations...)

	return stats
}

// Revoke withdraws every token the portal has issued, as an admin removing the
// CLI's access would. A revoked token is refused before it expires.
func (p *Portal) Revoke() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, token := range p.tokens {
		token.revoked = true
	}
}

// Decide acts as the user in a browser: it loads the verification page at
// pageURL, the pre-filled one the CLI opens, and submits decision on it.
//
// Returns [ErrVerificationRefused] when the page or the form answers anything
// but 200.
func Decide(ctx context.Context, c vhttp.Client, pageURL string, decision Decision) error {
	page, err := url.Parse(pageURL)
	if err != nil {
		return fmt.Errorf("parsing the verification URL: %w", err)
	}

	userCode := page.Query().Get(UserCodeParam)
	if userCode == "" {
		return fmt.Errorf("%w: the verification URL carries no %s", ErrVerificationRefused, UserCodeParam)
	}

	if err := send(ctx, c, http.MethodGet, pageURL, nil); err != nil {
		return err
	}

	form := url.Values{UserCodeParam: {userCode}, DecisionParam: {string(decision)}}
	action := page.ResolveReference(&url.URL{Path: VerificationPath}).String()

	return send(ctx, c, http.MethodPost, action, form)
}

func (p *Portal) authorize(w http.ResponseWriter, r *http.Request) {
	form, ok := p.readForm(w, r)
	if !ok {
		return
	}

	if form.Get("client_id") != ClientID {
		p.violatef("device authorization for client %q", form.Get("client_id"))
		p.writeError(w, http.StatusUnauthorized, ErrorInvalidClient)

		return
	}

	if form.Get("scope") != ScopeCatalogRead {
		p.violatef("device authorization for scope %q", form.Get("scope"))
		p.writeError(w, http.StatusBadRequest, ErrorInvalidScope)

		return
	}

	p.stats.Authorizations++

	n := p.stats.Authorizations
	deviceCode := "portaltest-device-code-" + strconv.Itoa(n)
	userCode := fmt.Sprintf("TEST-%04d", n)
	now := time.Now()

	req := &loginRequest{
		lastPoll:  now,
		expiresAt: now.Add(p.cfg.ExpiresIn),
		interval:  p.cfg.Interval,
	}
	p.requests[deviceCode] = req
	p.byUser[userCode] = req

	verificationURI := "https://" + r.Host + VerificationPath

	p.writeJSON(w, http.StatusOK, map[string]any{
		"device_code":               deviceCode,
		"user_code":                 userCode,
		"verification_uri":          verificationURI,
		"verification_uri_complete": verificationURI + "?" + url.Values{UserCodeParam: {userCode}}.Encode(),
		"expires_in":                int64(p.cfg.ExpiresIn / time.Second),
		"interval":                  int64(p.cfg.Interval / time.Second),
	})
}

func (p *Portal) token(w http.ResponseWriter, r *http.Request) {
	form, ok := p.readForm(w, r)
	if !ok {
		return
	}

	if form.Get("client_id") != ClientID {
		p.violatef("token poll for client %q", form.Get("client_id"))
		p.writeError(w, http.StatusUnauthorized, ErrorInvalidClient)

		return
	}

	if form.Get("grant_type") != DeviceCodeGrant {
		p.violatef("token poll with grant type %q", form.Get("grant_type"))
		p.writeError(w, http.StatusBadRequest, ErrorInvalidRequest)

		return
	}

	req, found := p.requests[form.Get("device_code")]
	if !found {
		p.violatef("token poll for a device code the portal never issued")
		p.writeError(w, http.StatusBadRequest, ErrorInvalidGrant)

		return
	}

	now := time.Now()

	if !now.Before(req.expiresAt) {
		p.writeError(w, http.StatusBadRequest, ErrorExpiredToken)

		return
	}

	switch req.state {
	case stateDenied:
		p.writeError(w, http.StatusBadRequest, ErrorAccessDenied)

		return
	case stateRedeemed:
		p.violatef("token poll for a device code already redeemed")
		p.writeError(w, http.StatusBadRequest, ErrorInvalidGrant)

		return
	case statePending, stateApproved:
	}

	tooFast := now.Sub(req.lastPoll) < req.interval
	req.lastPoll = now

	if tooFast {
		p.stats.TooFastPolls++
	}

	if tooFast || p.stats.SlowDowns < p.cfg.SlowDowns {
		req.interval += slowDownIncrement
		p.stats.SlowDowns++
		p.writeError(w, http.StatusBadRequest, ErrorSlowDown)

		return
	}

	if req.state == statePending {
		p.stats.PendingPolls++
		p.writeError(w, http.StatusBadRequest, ErrorAuthorizationPending)

		return
	}

	req.state = stateRedeemed
	p.stats.TokensIssued++

	accessToken := "portaltest-access-token-" + strconv.Itoa(p.stats.TokensIssued)
	p.tokens[accessToken] = &issuedToken{expiresAt: now.Add(p.cfg.TokenLifetime)}

	p.writeJSON(w, http.StatusOK, map[string]any{
		"access_token": accessToken,
		"token_type":   TokenTypeBearer,
		"expires_in":   int64(p.cfg.TokenLifetime / time.Second),
		"scope":        ScopeCatalogRead,
		"org_id":       p.cfg.OrgID,
		"org_name":     p.cfg.OrgName,
		"account":      map[string]string{"email": p.cfg.AccountEmail},
	})
}

func (p *Portal) verify(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		userCode := r.URL.Query().Get(UserCodeParam)
		if _, found := p.byUser[userCode]; !found {
			http.NotFound(w, r)

			return
		}

		w.Header().Set("Content-Type", htmlContentType)
		w.WriteHeader(http.StatusOK)

		if _, err := fmt.Fprintf(w, "<form method=\"post\" action=%q><input name=%q value=%q></form>",
			VerificationPath, UserCodeParam, html.EscapeString(userCode)); err != nil {
			p.t.Logf("portaltest: writing the verification page: %v", err)
		}
	case http.MethodPost:
		p.decide(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (p *Portal) decide(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	req, found := p.byUser[r.PostForm.Get(UserCodeParam)]
	if !found {
		http.NotFound(w, r)

		return
	}

	if !time.Now().Before(req.expiresAt) {
		w.WriteHeader(http.StatusGone)

		return
	}

	if req.state != statePending {
		w.WriteHeader(http.StatusConflict)

		return
	}

	switch Decision(r.PostForm.Get(DecisionParam)) {
	case Approve:
		req.state = stateApproved
	case Deny:
		req.state = stateDenied
	default:
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	w.WriteHeader(http.StatusOK)
}

func (p *Portal) catalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		p.violatef("catalog request with method %s", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)

		return
	}

	accessToken, hasBearer := strings.CutPrefix(r.Header.Get("Authorization"), bearerPrefix)
	token, found := p.tokens[accessToken]

	if !hasBearer || !found || token.revoked || !time.Now().Before(token.expiresAt) {
		p.stats.CatalogRejected++
		p.writeError(w, http.StatusUnauthorized, ErrorInvalidToken)

		return
	}

	p.stats.CatalogServed++

	repositories := make([]map[string]string, 0, len(p.cfg.Repositories))
	for _, repo := range p.cfg.Repositories {
		repositories = append(repositories, map[string]string{"url": repo})
	}

	p.writeJSON(w, http.StatusOK, map[string]any{"repositories": repositories})
}

// readForm reads the form body the OAuth endpoints take, recording a request
// that did not send one the way RFC 6749 §3.2 requires.
func (p *Portal) readForm(w http.ResponseWriter, r *http.Request) (url.Values, bool) {
	if r.Method != http.MethodPost {
		p.violatef("%s request to %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusMethodNotAllowed)

		return nil, false
	}

	if r.Header.Get("Content-Type") != formContentType {
		p.violatef("%s body sent as %q", r.URL.Path, r.Header.Get("Content-Type"))
		p.writeError(w, http.StatusBadRequest, ErrorInvalidRequest)

		return nil, false
	}

	if err := r.ParseForm(); err != nil {
		p.violatef("unreadable %s body: %v", r.URL.Path, err)
		p.writeError(w, http.StatusBadRequest, ErrorInvalidRequest)

		return nil, false
	}

	return r.PostForm, true
}

func (p *Portal) violatef(format string, args ...any) {
	p.stats.Violations = append(p.stats.Violations, fmt.Sprintf(format, args...))
}

func (p *Portal) writeError(w http.ResponseWriter, status int, code string) {
	p.writeJSON(w, status, map[string]string{"error": code})
}

func (p *Portal) writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(body); err != nil {
		p.t.Logf("portaltest: writing a response: %v", err)
	}
}

// send makes one request as the user's browser, reporting a page that did not
// answer 200 as [ErrVerificationRefused].
func send(ctx context.Context, c vhttp.Client, method, target string, form url.Values) error {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}

	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return fmt.Errorf("building the verification page request: %w", err)
	}

	if form != nil {
		req.Header.Set("Content-Type", formContentType)
	}

	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("reaching the verification page: %w", err)
	}

	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return errors.Join(fmt.Errorf("reading the verification page: %w", err), resp.Body.Close())
	}

	if err := resp.Body.Close(); err != nil {
		return fmt.Errorf("closing the verification page: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %s %s answered %d", ErrVerificationRefused, method, target, resp.StatusCode)
	}

	return nil
}
