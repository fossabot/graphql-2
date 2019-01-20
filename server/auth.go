package main

import (
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/gofrs/uuid"
	"github.com/icco/graphql"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/plus/v1"
)

const (
	googleProfileSessionKey = "google_profile"
	oauthTokenSessionKey    = "oauth_token"
	oauthFlowRedirectKey    = "redirect"
)

var (
	// OAuthConfig is used to store and share the Oauth2 Config.
	OAuthConfig *oauth2.Config
)

func appErrorf(w http.ResponseWriter, err error, msg string, args ...interface{}) {
	message := fmt.Sprintf(msg, args...)
	log.WithError(err).Error(message)
	j, _ := json.Marshal(map[string]string{"error": message})
	http.Error(w, string(j), http.StatusInternalServerError)
	return
}

func validateRedirectURL(path string) (string, error) {
	if path == "" {
		return "/", nil
	}

	// Ensure redirect URL is valid and not pointing to a different server.
	parsedURL, err := url.Parse(path)
	if err != nil {
		return "/", err
	}
	if parsedURL.IsAbs() {
		return "/", fmt.Errorf("URL must not be absolute")
	}
	return path, nil
}

func configureOAuthClient(clientID, clientSecret, redirectURL string) *oauth2.Config {
	if redirectURL == "" {
		redirectURL = "http://localhost:8080/oauth2callback"
	}
	return &oauth2.Config{
		ClientID:     strings.TrimSpace(clientID),
		ClientSecret: strings.TrimSpace(clientSecret),
		RedirectURL:  strings.TrimSpace(redirectURL),
		Scopes: []string{
			plus.PlusMeScope,
			plus.UserinfoEmailScope,
			plus.UserinfoProfileScope,
		},
		Endpoint: google.Endpoint,
	}
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	// TODO: Actually log folks out.
	http.Redirect(w, r, "/", http.StatusFound)
}

func callbackHandler(w http.ResponseWriter, r *http.Request) {
	oauthFlowSession, err := SessionStore.Get(r, r.FormValue("state"))
	if err != nil {
		appErrorf(w, err, "invalid state parameter. try logging in again.")
		return
	}

	redirectURL, ok := oauthFlowSession.Values[oauthFlowRedirectKey].(string)
	// Validate this callback request came from the app.
	if !ok {
		appErrorf(w, err, "invalid state parameter. try logging in again.")
		return
	}

	code := r.FormValue("code")
	tok, err := OAuthConfig.Exchange(context.Background(), code)
	if err != nil {
		appErrorf(w, err, "could not get auth token: %v", err)
		return
	}

	session, err := SessionStore.New(r, defaultSessionID)
	if err != nil {
		appErrorf(w, err, "could not get default session: %v", err)
		return
	}

	client := oauth2.NewClient(r.Context(), OAuthConfig.TokenSource(r.Context(), tok))
	plusService, err := plus.New(client)
	if err != nil {
		appErrorf(w, err, "could not get plus api: %v", err)
		return
	}
	profile, err := plusService.People.Get("me").Do()
	if err != nil {
		appErrorf(w, err, "could not fetch Google profile: %v", err)
		return
	}

	user, err := graphql.GetUser(r.Context(), profile.Id)
	if err != nil {
		appErrorf(w, err, "could not upsert user: %v", err)
		return
	}
	log.Printf("user: %+v", user)

	// Actually save something to session
	session.Values[oauthTokenSessionKey] = tok
	session.Values[googleProfileSessionKey] = user
	if err := session.Save(r, w); err != nil {
		appErrorf(w, err, "could not save session: %v", err)
		return
	}

	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := uuid.Must(uuid.NewV4()).String()

	oauthFlowSession, err := SessionStore.New(r, sessionID)
	if err != nil {
		appErrorf(w, err, "could not create oauth session: %v", err)
		return
	}
	oauthFlowSession.Options.MaxAge = 10 * 60 // 10 minutes

	redirectURL, err := validateRedirectURL(r.FormValue("redirect"))
	if err != nil {
		appErrorf(w, err, "invalid redirect URL: %v", err)
		return
	}
	oauthFlowSession.Values[oauthFlowRedirectKey] = redirectURL

	if err := oauthFlowSession.Save(r, w); err != nil {
		appErrorf(w, err, "could not save session: %v", err)
		return
	}

	url := OAuthConfig.AuthCodeURL(sessionID, oauth2.ApprovalForce, oauth2.AccessTypeOnline)
	http.Redirect(w, r, url, http.StatusFound)
}
