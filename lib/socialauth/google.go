package socialauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const (
	googleAuthorizeURL = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL     = "https://oauth2.googleapis.com/token"
	googleUserInfoURL  = "https://openidconnect.googleapis.com/v1/userinfo"
)

// Google implements Provider against Google's OAuth2 / OpenID Connect.
type Google struct {
	clientID string
	secret   string
	scopes   []string
}

// NewGoogle builds a Google provider. Default scopes request the OpenID
// profile and email.
func NewGoogle(clientID, secret string, scopes []string) *Google {
	if len(scopes) == 0 {
		scopes = []string{"openid", "email", "profile"}
	}
	return &Google{clientID: clientID, secret: secret, scopes: scopes}
}

func (g *Google) Name() string { return "google" }

func (g *Google) AuthCodeURL(state, redirectURI string) string {
	q := url.Values{}
	q.Set("client_id", g.clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("scope", strings.Join(g.scopes, " "))
	q.Set("state", state)
	q.Set("access_type", "online")
	return googleAuthorizeURL + "?" + q.Encode()
}

func (g *Google) Authenticate(ctx context.Context, code, redirectURI string) (*UserProfile, error) {
	token, err := g.exchange(ctx, code, redirectURI)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleUserInfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google: userinfo request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google: userinfo status %d", resp.StatusCode)
	}

	var info struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("google: decode userinfo: %w", err)
	}
	if info.Sub == "" {
		return nil, errors.New("google: empty subject")
	}
	if !info.EmailVerified || info.Email == "" {
		return nil, errors.New("google: account has no verified email")
	}
	return &UserProfile{
		Subject:   info.Sub,
		Email:     info.Email,
		Name:      info.Name,
		AvatarURL: info.Picture,
	}, nil
}

func (g *Google) exchange(ctx context.Context, code, redirectURI string) (string, error) {
	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", g.clientID)
	form.Set("client_secret", g.secret)
	form.Set("redirect_uri", redirectURI)
	form.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("google: token request: %w", err)
	}
	defer resp.Body.Close()

	var body struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("google: decode token: %w", err)
	}
	if body.Error != "" {
		return "", fmt.Errorf("google: token error: %s", body.Error)
	}
	if body.AccessToken == "" {
		return "", errors.New("google: empty access token")
	}
	return body.AccessToken, nil
}
