package oauth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
)

const AuthorizeEndpoint = "https://app.asana.com/-/oauth_authorize"
const DefaultRedirectURI = "http://127.0.0.1:18787/callback"

var DefaultScopes = []string{"default"}
var DefaultOAuthScopes = DefaultScopes

func GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

type AuthURLParams struct {
	ClientID    string
	RedirectURI string
	Scopes      []string
	State       string
}

func BuildAuthURL(p AuthURLParams) string {
	redirect := p.RedirectURI
	if redirect == "" {
		redirect = DefaultRedirectURI
	}
	scopes := p.Scopes
	if len(scopes) == 0 {
		scopes = DefaultScopes
	}
	v := url.Values{}
	v.Set("client_id", p.ClientID)
	v.Set("redirect_uri", redirect)
	v.Set("response_type", "code")
	v.Set("scope", strings.Join(scopes, " "))
	if p.State != "" {
		v.Set("state", p.State)
	}
	return AuthorizeEndpoint + "?" + strings.ReplaceAll(v.Encode(), "+", "%20")
}

func BuildAuthorizationURL(clientID, redirectURI, state string, scopes []string) (string, error) {
	return BuildAuthURL(AuthURLParams{ClientID: clientID, RedirectURI: redirectURI, State: state, Scopes: scopes}), nil
}

func ValidateRedirectURI(raw string) error { return ValidateLocalRedirect(raw) }

func ValidateLocalRedirect(raw string) error {
	if strings.EqualFold(raw, "urn:ietf:wg:oauth:2.0:oob") || strings.EqualFold(raw, "oob") {
		return fmt.Errorf("OOB redirect URI is not supported")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "http" {
		return fmt.Errorf("redirect URI scheme must be http")
	}
	host := u.Hostname()
	if host != "127.0.0.1" && host != "localhost" {
		return fmt.Errorf("redirect URI host must be localhost or 127.0.0.1")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("redirect URI must not include query or fragment")
	}
	if u.Path == "" {
		return fmt.Errorf("redirect URI must include a path")
	}
	return nil
}
