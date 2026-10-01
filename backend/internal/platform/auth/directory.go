package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	directorysdk "go.cntt.one/product/directory"
)

// LoginProvider is what the login handlers need from the company identity
// system, kept as an interface so tests can stand in a fake instead of
// dialling the real thing -- Directory talks a persistent QUIC connection,
// not request/response HTTP, so there is no httptest.Server to fake it with.
type LoginProvider interface {
	AuthorizeURL() (string, error)
	Identity(ctx context.Context, code string) (OIDCIdentity, error)
	Secure() bool
}

// Directory is the company login flow: a persistent connection to the
// Directory server, authenticated with an application Key/Secret rather than
// a user credential. AuthorizeUrl/Redeem replace OIDC's authorization-code
// redirect and token exchange; there is no PKCE (the protocol has no
// concept of one) and no RP-initiated logout endpoint (see AuthLogout).
type Directory struct {
	client      *directorysdk.Directory
	redirectURL string

	// secure is whether FRONTEND_URL is https, i.e. whether the cookies this
	// deployment sets must carry Secure whatever the proxy in front of the API
	// claims the scheme is. X-Forwarded-Proto is only as reliable as whoever
	// configured the ingress; FRONTEND_URL is this deployment's own statement
	// of its public URL and has to be right for the redirect to work at all.
	secure bool
}

type OIDCIdentity struct {
	Subject string
	Display string
	Email   string
	Picture string
}

// A colon cannot occur in a LABEL_TOOL_USERS username because it is that
// config format's name/hash delimiter, so a directory session cannot
// collide with a valid legacy account.
const oidcSessionPrefix = "dir:"

// NewDirectory returns nil, nil when the Directory login is not configured at
// all (the local-account path), and an error when it is only partially
// configured -- a deployment missing one of these three variables should fail
// loudly at startup, not silently fall back to local accounts.
//
// onProfileUpdate is called for every Client.Update push the Directory server
// sends over the connection (a name or email changed on their end); nil skips
// the sync. It is best-effort by design -- a push that cannot be recorded is
// a reporting problem, not a reason to NAK the update (see Config.OnUpdate).
func NewDirectory(address, key, secret, frontendURL string, onProfileUpdate func(ctx context.Context, identity OIDCIdentity)) (*Directory, error) {
	values := map[string]string{
		"DIRECTORY_ADDRESS": address, "DIRECTORY_KEY": key, "DIRECTORY_SECRET": secret,
		"FRONTEND_URL": frontendURL,
	}
	configured := address != "" || key != "" || secret != ""
	if !configured {
		return nil, nil
	}
	for name, value := range values {
		if value == "" {
			return nil, fmt.Errorf("%s is required when the Directory login is configured", name)
		}
	}

	redirectURL, err := url.JoinPath(frontendURL, "/entry/callback")
	if err != nil {
		return nil, fmt.Errorf("directory redirect URL: %w", err)
	}

	client, err := directorysdk.New(&directorysdk.Config{
		Address: address, Key: key, Secret: secret,
		OnUpdate: func(ctx context.Context, request *directorysdk.UpdateRequest) error {
			if request.User == nil {
				return nil
			}
			identity, err := identityFromUser(request.User)
			if err != nil {
				// A push with no usable subject is not this deployment's problem to
				// reject -- ack it and move on rather than jamming the connection.
				return nil
			}
			if onProfileUpdate != nil {
				onProfileUpdate(ctx, identity)
			}
			return nil
		},
	})
	if err != nil {
		return nil, fmt.Errorf("directory client: %w", err)
	}

	return &Directory{
		client:      client,
		redirectURL: redirectURL,
		secure:      strings.HasPrefix(strings.ToLower(frontendURL), "https://"),
	}, nil
}

// Start opens the connection and keeps it alive (auto-redialing) for the
// life of the process. Close on shutdown.
func (d *Directory) Start() error { return d.client.Start() }
func (d *Directory) Close() error { return d.client.Close() }

func (d *Directory) Secure() bool { return d.secure }

// AuthorizeURL builds the directory's login page URL. redirect must match a
// URI pre-registered with the directory application exactly -- tried
// appending our own CSRF state as a query string here first, and the
// directory rejects the whole link for it (APPLICATION-REDIRECT-URI-FORBIDDEN,
// found the hard way). So the protocol gets no state at all: CSRF protection
// lives entirely in loginStateCookie, whose mere presence at the callback is
// the proof this browser is the one that started the flow.
func (d *Directory) AuthorizeURL() (string, error) {
	authorizeURL, err := d.client.AuthorizeUrl(d.redirectURL)
	if err != nil {
		return "", fmt.Errorf("directory authorize URL: %w", err)
	}
	return authorizeURL, nil
}

// Identity redeems the one-use code entirely on the server, over the
// already-open connection. Directory credentials never become a response
// body, browser cookie, localStorage value, or log field.
func (d *Directory) Identity(ctx context.Context, code string) (OIDCIdentity, error) {
	kind := directorysdk.RedeemKindWeb
	redeemed, err := d.client.Redeem(ctx, &directorysdk.RedeemRequest{Code: &code, Kind: &kind})
	if err != nil {
		return OIDCIdentity{}, fmt.Errorf("redeem code: %w", err)
	}
	if redeemed.User == nil {
		return OIDCIdentity{}, fmt.Errorf("directory redeem returned no user")
	}
	return identityFromUser(redeemed.User)
}

func identityFromUser(u *directorysdk.User) (OIDCIdentity, error) {
	subject := strings.TrimSpace(deref(u.Id))
	if subject == "" {
		return OIDCIdentity{}, fmt.Errorf("directory user has no id")
	}
	email := strings.TrimSpace(deref(u.Email))
	picture := strings.TrimSpace(deref(u.Picture))
	for _, display := range []string{deref(u.Username), email} {
		if display = strings.TrimSpace(display); display != "" {
			return OIDCIdentity{Subject: subject, Display: display, Email: email, Picture: picture}, nil
		}
	}
	return OIDCIdentity{Subject: subject, Display: subject, Email: email, Picture: picture}, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// OIDCSessionIdentity keeps the stable subject, human display name, and
// avatar URL in the signed application session. Local-login session values
// remain unchanged. The picture rides in the cookie rather than a DB column
// -- it is only ever read back for the signed-in caller's own avatar, so
// there is nothing to join against, and it goes stale the same way Display
// already does: on next login, not before.
func OIDCSessionIdentity(identity OIDCIdentity) string {
	raw, _ := json.Marshal([3]string{identity.Subject, identity.Display, identity.Picture})
	return oidcSessionPrefix + base64.RawURLEncoding.EncodeToString(raw)
}

// SessionIdentity decodes a session value. picture is "" for local logins and
// for any directory session recorded before this field existed -- a fixed-size
// Go array decodes a shorter JSON array by zeroing what's missing, so an old
// two-element cookie is read as "no picture", not a decode error.
func SessionIdentity(value string) (attribution, display, picture string, isDirectory bool) {
	encoded, found := strings.CutPrefix(value, oidcSessionPrefix)
	if !found {
		return value, value, "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return value, value, "", false
	}
	var identity [3]string
	if json.Unmarshal(raw, &identity) != nil || identity[0] == "" || identity[1] == "" {
		return value, value, "", false
	}
	return identity[0], identity[1], identity[2], true
}
