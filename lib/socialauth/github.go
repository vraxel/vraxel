package socialauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	githubAuthorizeURL = "https://github.com/login/oauth/authorize"
	githubTokenURL     = "https://github.com/login/oauth/access_token"
	githubUserURL      = "https://api.github.com/user"
	githubEmailsURL    = "https://api.github.com/user/emails"
)

// GitHub implements Provider against GitHub's OAuth apps.
type GitHub struct {
	clientID string
	secret   string
	scopes   []string
}

// NewGitHub builds a GitHub provider. Default scopes read the profile and the
// verified email addresses.
func NewGitHub(clientID, secret string, scopes []string) *GitHub {
	if len(scopes) == 0 {
		scopes = []string{"read:user", "user:email"}
	}
	return &GitHub{clientID: clientID, secret: secret, scopes: scopes}
}

func (g *GitHub) Name() string { return "github" }

func (g *GitHub) AuthCodeURL(state, redirectURI string) string {
	q := url.Values{}
	q.Set("client_id", g.clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", strings.Join(g.scopes, " "))
	q.Set("state", state)
	return githubAuthorizeURL + "?" + q.Encode()
}

func (g *GitHub) Authenticate(ctx context.Context, code, redirectURI string) (*UserProfile, error) {
	token, err := g.exchange(ctx, code, redirectURI)
	if err != nil {
		return nil, err
	}

	var user struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := githubGet(ctx, githubUserURL, token, &user); err != nil {
		return nil, fmt.Errorf("github: fetch user: %w", err)
	}
	if user.ID == 0 {
		return nil, errors.New("github: empty user id")
	}

	email, err := g.primaryVerifiedEmail(ctx, token)
	if err != nil {
		return nil, err
	}

	name := user.Name
	if name == "" {
		name = user.Login
	}
	return &UserProfile{
		Subject:   strconv.FormatInt(user.ID, 10),
		Email:     email,
		Name:      name,
		AvatarURL: user.AvatarURL,
	}, nil
}

func (g *GitHub) exchange(ctx context.Context, code, redirectURI string) (string, error) {
	form := url.Values{}
	form.Set("client_id", g.clientID)
	form.Set("client_secret", g.secret)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, githubTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("github: token request: %w", err)
	}
	defer resp.Body.Close()

	var body struct {
		AccessToken      string `json:"access_token"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("github: decode token: %w", err)
	}
	if body.Error != "" {
		return "", fmt.Errorf("github: token error: %s", body.Error)
	}
	if body.AccessToken == "" {
		return "", errors.New("github: empty access token")
	}
	return body.AccessToken, nil
}

// primaryVerifiedEmail returns the account's primary, verified email. GitHub
// only lets us provision accounts we can trust the address of, so an account
// with no verified primary email is rejected.
func (g *GitHub) primaryVerifiedEmail(ctx context.Context, token string) (string, error) {
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := githubGet(ctx, githubEmailsURL, token, &emails); err != nil {
		return "", fmt.Errorf("github: fetch emails: %w", err)
	}
	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}
	return "", errors.New("github: no verified primary email on account")
}

func githubGet(ctx context.Context, endpoint, token string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
