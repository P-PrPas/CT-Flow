package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// OIDC is the company login flow used by corpus-core: authorization-code
// redirect, server-side token exchange, then user-info claim mapping.
type OIDC struct {
	Provider *oidc.Provider
	OAuth2   oauth2.Config

	// PKCE is off unless the discovery document advertises S256. Sending a
	// code_challenge a provider never asked for is how you find out at 3am
	// that it rejects unknown parameters, and this flow already has a
	// confidential client -- PKCE is defence in depth here, not the thing
	// holding it up.
	PKCE bool

	// EndSession is the provider's RP-initiated logout endpoint, "" when the
	// discovery document has none. Without it "sign out" only clears the
	// CT-Flow cookie, and on the shared labelling machine this tool is
	// deployed on (LABEL_TOOL_MODE=vm) the next click on "sign in" walks
	// straight back in as the same person with no prompt.
	EndSession string

	// Secure is whether FRONTEND_URL is https, i.e. whether the cookies this
	// deployment sets must carry Secure whatever the proxy in front of the API
	// claims the scheme is. X-Forwarded-Proto is only as reliable as whoever
	// configured the ingress; FRONTEND_URL is this deployment's own statement
	// of its public URL and has to be right for the redirect to work at all.
	Secure bool
}

type oidcClaims struct {
	ID       string `json:"sub"`
	Username string `json:"preferred_username"`
	Email    string `json:"email"`
}

type OIDCIdentity struct {
	Subject string
	Display string
	Email   string
}

// A colon cannot occur in a LABEL_TOOL_USERS username because it is that
// config format's name/hash delimiter, so OIDC sessions cannot collide with a
// valid legacy account.
const oidcSessionPrefix = "oidc:"

func NewOIDC(ctx context.Context, clientID, clientSecret, issuer, frontendURL string) (*OIDC, error) {
	values := map[string]string{
		"OAUTH_CLIENT_ID": clientID, "OAUTH_CLIENT_SECRET": clientSecret,
		"OAUTH_ENDPOINT": issuer, "FRONTEND_URL": frontendURL,
	}
	configured := clientID != "" || clientSecret != "" || issuer != ""
	if !configured {
		return nil, nil
	}
	for name, value := range values {
		if value == "" {
			return nil, fmt.Errorf("%s is required when OIDC is configured", name)
		}
	}

	redirectURL, err := url.JoinPath(frontendURL, "/entry/callback")
	if err != nil {
		return nil, fmt.Errorf("OIDC redirect URL: %w", err)
	}
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("OIDC discovery: %w", err)
	}
	// Both of these are optional in the discovery document, so a provider that
	// omits them turns the feature off rather than failing the login. The error
	// is ignored for the same reason: no extra claims is not a broken provider.
	var meta struct {
		EndSession    string   `json:"end_session_endpoint"`
		CodeChallenge []string `json:"code_challenge_methods_supported"`
	}
	_ = provider.Claims(&meta)

	return &OIDC{
		Provider: provider,
		OAuth2: oauth2.Config{
			ClientID: clientID, ClientSecret: clientSecret, RedirectURL: redirectURL,
			Endpoint: provider.Endpoint(),
			Scopes:   []string{oidc.ScopeOpenID, "email", "profile"},
		},
		PKCE:       slices.Contains(meta.CodeChallenge, "S256"),
		EndSession: meta.EndSession,
		Secure:     strings.HasPrefix(strings.ToLower(frontendURL), "https://"),
	}, nil
}

func (o *OIDC) AuthCodeURL(state, verifier string) string {
	if !o.PKCE {
		return o.OAuth2.AuthCodeURL(state)
	}
	return o.OAuth2.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))
}

// Identity exchanges the one-use code entirely on the server. Provider tokens
// never become a response body, browser cookie, localStorage value, or log field.
func (o *OIDC) Identity(ctx context.Context, code, verifier string) (OIDCIdentity, error) {
	var opts []oauth2.AuthCodeOption
	if o.PKCE {
		opts = append(opts, oauth2.VerifierOption(verifier))
	}
	token, err := o.OAuth2.Exchange(ctx, code, opts...)
	if err != nil {
		return OIDCIdentity{}, fmt.Errorf("exchange code: %w", err)
	}
	info, err := o.Provider.UserInfo(ctx, oauth2.StaticTokenSource(token))
	if err != nil {
		return OIDCIdentity{}, fmt.Errorf("get user info: %w", err)
	}
	var claims oidcClaims
	if err := info.Claims(&claims); err != nil {
		return OIDCIdentity{}, fmt.Errorf("parse user claims: %w", err)
	}
	claims.ID = strings.TrimSpace(claims.ID)
	if claims.ID == "" {
		return OIDCIdentity{}, fmt.Errorf("OIDC user info has no sub")
	}
	email := strings.TrimSpace(claims.Email)
	for _, display := range []string{claims.Username, email} {
		if display = strings.TrimSpace(display); display != "" {
			return OIDCIdentity{Subject: claims.ID, Display: display, Email: email}, nil
		}
	}
	return OIDCIdentity{Subject: claims.ID, Display: claims.ID, Email: email}, nil
}

// OIDCSessionIdentity keeps the stable subject and human display name in the
// signed application session. Local-login session values remain unchanged.
func OIDCSessionIdentity(identity OIDCIdentity) string {
	raw, _ := json.Marshal([2]string{identity.Subject, identity.Display})
	return oidcSessionPrefix + base64.RawURLEncoding.EncodeToString(raw)
}

func SessionIdentity(value string) (attribution, display string, oidc bool) {
	encoded, found := strings.CutPrefix(value, oidcSessionPrefix)
	if !found {
		return value, value, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return value, value, false
	}
	var identity [2]string
	if json.Unmarshal(raw, &identity) != nil || identity[0] == "" || identity[1] == "" {
		return value, value, false
	}
	return identity[0], identity[1], true
}
