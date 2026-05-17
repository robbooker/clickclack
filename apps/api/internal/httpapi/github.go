package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/openclaw/clickclack/apps/api/internal/store"
)

type GitHubOAuthConfig struct {
	ClientID      string
	ClientSecret  string
	PublicURL     string
	AuthURL       string
	TokenURL      string
	UserURL       string
	EmailsURL     string
	MembershipURL string
	AllowedOrg    string
	HTTPClient    *http.Client
}

var errGitHubOrgDenied = errors.New("github account is not a member of the allowed organization")

const defaultGitHubHTTPTimeout = 30 * time.Second

func (c GitHubOAuthConfig) withDefaults() GitHubOAuthConfig {
	if c.AuthURL == "" {
		c.AuthURL = "https://github.com/login/oauth/authorize"
	}
	if c.TokenURL == "" {
		c.TokenURL = "https://github.com/login/oauth/access_token"
	}
	if c.UserURL == "" {
		c.UserURL = "https://api.github.com/user"
	}
	if c.EmailsURL == "" {
		c.EmailsURL = "https://api.github.com/user/emails"
	}
	if c.MembershipURL == "" {
		c.MembershipURL = "https://api.github.com/user/memberships/orgs/"
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: defaultGitHubHTTPTimeout}
	}
	return c
}

func (s *Server) githubStart(w http.ResponseWriter, r *http.Request) {
	if s.githubOAuth.ClientID == "" || s.githubOAuth.ClientSecret == "" {
		writeError(w, http.StatusNotImplemented, errors.New("github oauth is not configured"))
		return
	}
	state, err := randomToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "cc_github_state", Value: state, Path: "/", MaxAge: 600, HttpOnly: true, Secure: s.secureCookies(r), SameSite: http.SameSiteLaxMode})
	values := url.Values{
		"client_id":    {s.githubOAuth.ClientID},
		"redirect_uri": {s.githubRedirectURL(r)},
		"scope":        {s.githubScope()},
		"state":        {state},
	}
	http.Redirect(w, r, s.githubOAuth.AuthURL+"?"+values.Encode(), http.StatusFound)
}

func (s *Server) githubCallback(w http.ResponseWriter, r *http.Request) {
	state, err := r.Cookie("cc_github_state")
	if err != nil || state.Value == "" || state.Value != r.URL.Query().Get("state") {
		writeError(w, http.StatusBadRequest, errors.New("invalid github oauth state"))
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		writeError(w, http.StatusBadRequest, errors.New("github oauth code is required"))
		return
	}
	token, err := s.exchangeGitHubCode(r.Context(), r, code)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	profile, err := s.fetchGitHubProfile(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if err := s.ensureGitHubOrgMembership(r.Context(), token); err != nil {
		if errors.Is(err, errGitHubOrgDenied) {
			writeError(w, http.StatusForbidden, err)
			return
		}
		writeError(w, http.StatusBadGateway, err)
		return
	}
	user, err := s.store.UpsertIdentityUser(r.Context(), store.UpsertIdentityUserInput{
		Provider:        "github",
		ProviderSubject: strconv.FormatInt(profile.ID, 10),
		Email:           profile.Email,
		DisplayName:     firstNonEmpty(profile.Name, profile.Login, profile.Email),
		AvatarURL:       profile.AvatarURL,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(s.githubOAuth.AllowedOrg) != "" {
		if _, err := s.store.EnsureDefaultWorkspaceMember(r.Context(), user.ID); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	session, err := s.store.CreateSession(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.setSessionCookie(w, r, session)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) exchangeGitHubCode(ctx context.Context, r *http.Request, code string) (string, error) {
	body := url.Values{
		"client_id":     {s.githubOAuth.ClientID},
		"client_secret": {s.githubOAuth.ClientSecret},
		"code":          {code},
		"redirect_uri":  {s.githubRedirectURL(r)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.githubOAuth.TokenURL, strings.NewReader(body.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.githubOAuth.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", errors.New("github token exchange failed")
	}
	var out struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.Error != "" {
		return "", errors.New(out.Error)
	}
	if out.AccessToken == "" {
		return "", errors.New("github access token missing")
	}
	return out.AccessToken, nil
}

type githubProfile struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

func (s *Server) fetchGitHubProfile(ctx context.Context, token string) (githubProfile, error) {
	var profile githubProfile
	if err := s.githubGetJSON(ctx, s.githubOAuth.UserURL, token, &profile); err != nil {
		return githubProfile{}, err
	}
	if profile.ID == 0 {
		return githubProfile{}, errors.New("github profile id missing")
	}
	if profile.Email == "" {
		var emails []struct {
			Email   string `json:"email"`
			Primary bool   `json:"primary"`
		}
		if err := s.githubGetJSON(ctx, s.githubOAuth.EmailsURL, token, &emails); err != nil {
			return githubProfile{}, err
		}
		for _, item := range emails {
			if item.Primary {
				profile.Email = item.Email
				break
			}
		}
	}
	return profile, nil
}

func (s *Server) githubGetJSON(ctx context.Context, endpoint, token string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := s.githubOAuth.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return errors.New("github api request failed")
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (s *Server) ensureGitHubOrgMembership(ctx context.Context, token string) error {
	org := strings.TrimSpace(s.githubOAuth.AllowedOrg)
	if org == "" {
		return nil
	}
	endpoint := strings.TrimRight(s.githubOAuth.MembershipURL, "/") + "/" + url.PathEscape(org)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := s.githubOAuth.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden {
		return errGitHubOrgDenied
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("github organization membership check failed: %s", resp.Status)
	}
	var membership struct {
		State        string `json:"state"`
		Organization struct {
			Login string `json:"login"`
		} `json:"organization"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&membership); err != nil {
		return err
	}
	if !strings.EqualFold(membership.State, "active") || !strings.EqualFold(membership.Organization.Login, org) {
		return errGitHubOrgDenied
	}
	return nil
}

func (s *Server) githubScope() string {
	scope := "read:user user:email"
	if strings.TrimSpace(s.githubOAuth.AllowedOrg) != "" {
		scope += " read:org"
	}
	return scope
}

func (s *Server) githubRedirectURL(r *http.Request) string {
	base := strings.TrimRight(s.githubOAuth.PublicURL, "/")
	if base == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		base = scheme + "://" + r.Host
	}
	return base + "/api/auth/github/callback"
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, session store.Session) {
	expires, _ := time.Parse(time.RFC3339Nano, session.ExpiresAt)
	http.SetCookie(w, &http.Cookie{Name: "cc_session", Value: session.Token, Path: "/", Expires: expires, HttpOnly: true, Secure: s.secureCookies(r), SameSite: http.SameSiteLaxMode})
}

func (s *Server) secureCookies(r *http.Request) bool {
	if publicURL, err := url.Parse(strings.TrimSpace(s.githubOAuth.PublicURL)); err == nil && publicURL.Scheme == "https" {
		return true
	}
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func randomToken() (string, error) {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(data[:]), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
