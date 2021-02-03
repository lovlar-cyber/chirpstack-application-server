package oidc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/coreos/go-oidc"
	"github.com/dgrijalva/jwt-go"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"golang.org/x/oauth2"

	"github.com/lovlar-cyber/chirpstack-application-server/internal/config"
)

var (
	providerURL  string
	clientID     string
	clientSecret string
	redirectURL  string
	jwtSecret    string

	// MockGetUserUser contains a possible mocked GetUser User
	MockGetUserUser *User
	// MockGetUserError contains a possible mocked GetUser error
	MockGetUserError error
)

// User defines an OpenID Connect user object.
type User struct {
	ExternalID    string `json:"sub"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
}

// Setup configured the OpenID Connect endpoint handlers.
func Setup(conf config.Config, r *mux.Router) error {
	oidcConfig := conf.ApplicationServer.UserAuthentication.OpenIDConnect
	externalAPIConfig := conf.ApplicationServer.ExternalAPI

	if !oidcConfig.Enabled {
		return nil
	}

	log.WithFields(log.Fields{
		"login": "/auth/oidc/login",
	}).Info("oidc: setting up openid connect endpoints")

	providerURL = oidcConfig.ProviderURL
	clientID = oidcConfig.ClientID
	clientSecret = oidcConfig.ClientSecret
	redirectURL = oidcConfig.RedirectURL
	jwtSecret = externalAPIConfig.JWTSecret

	r.HandleFunc("/auth/oidc/login", loginHandler)
	r.HandleFunc("/auth/oidc/callback", callbackHandler)

	return nil
}

type authenticator struct {
	provider *oidc.Provider
	config   oauth2.Config
}

func newAuthenticator(ctx context.Context) (*authenticator, error) {
	if providerURL == "" || clientID == "" || clientSecret == "" || redirectURL == "" {
		return nil, errors.New("openid connect is not properly configured")
	}

	provider, err := oidc.NewProvider(ctx, providerURL)
	if err != nil {
		return nil, errors.Wrap(err, "get provider error")
	}

	conf := oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}

	return &authenticator{
		provider: provider,
		config:   conf,
	}, nil
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	// get state
	state, err := getState()
	if err != nil {
		http.Error(w, "get state error", http.StatusInternalServerError)
		log.WithError(err).Error("oidc: get state error")
		return
	}

	// get authenticator
	auth, err := newAuthenticator(r.Context())
	if err != nil {
		http.Error(w, "get authenticator error", http.StatusInternalServerError)
		log.WithError(err).Error("oidc: new authenticator error")
		return
	}

	http.Redirect(w, r, auth.config.AuthCodeURL(state), http.StatusFound)
}

func callbackHandler(w http.ResponseWriter, r *http.Request) {
	// redirect to web-interface, which will use a gRPC call to handle the
	// login.
	redirect := fmt.Sprintf("/#/login?code=%s&state=%s",
		r.URL.Query().Get("code"),
		r.URL.Query().Get("state"),
	)

	http.Redirect(w, r, redirect, http.StatusPermanentRedirect)
}

func getState() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Reader.Read(b)
	if err != nil {
		return "", errors.Wrap(err, "read random bytes error")
	}
	state := base64.StdEncoding.EncodeToString(b)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.StandardClaims{
		NotBefore: time.Now().Unix(),
		ExpiresAt: time.Now().Add(5 * time.Minute).Unix(),
		Id:        state,
	})

	return token.SignedString([]byte(jwtSecret))
}

func validateState(state string) (bool, error) {
	token, err := jwt.Parse(state, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return false, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		return []byte(jwtSecret), nil
	})
	if err != nil {
		return false, errors.Wrap(err, "parse state error")
	}

	return token.Valid, nil
}

// GetUser returns the OpenID Connect user object for the given code and state.
func GetUser(ctx context.Context, code string, state string) (User, error) {
	// for testing the API
	if MockGetUserUser != nil {
		return *MockGetUserUser, MockGetUserError
	}

	ok, err := validateState(state)
	if err != nil {
		return User{}, errors.Wrap(err, "validate state error")
	}
	if !ok {
		return User{}, errors.New("state is invalid or has expired")
	}

	auth, err := newAuthenticator(ctx)
	if err != nil {
		return User{}, errors.Wrap(err, "new oidc authenticator error")
	}

	token, err := auth.config.Exchange(ctx, code)
	if err != nil {
		return User{}, errors.Wrap(err, "exchange oidc token error")
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return User{}, errors.Wrap(err, "no id_token field in oauth2 token")
	}

	oidcConfig := &oidc.Config{
		ClientID: clientID,
	}

	idToken, err := auth.provider.Verifier(oidcConfig).Verify(ctx, rawIDToken)
	if err != nil {
		return User{}, errors.Wrap(err, "verify id token error")
	}

	var user User
	if err := idToken.Claims(&user); err != nil {
		return User{}, errors.Wrap(err, "get userInfo error")
	}

	return user, nil
}
