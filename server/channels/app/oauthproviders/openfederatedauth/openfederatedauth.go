// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package openfederatedauth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/mattermost/mattermost/server/public/shared/request"
	"github.com/mattermost/mattermost/server/v8/einterfaces"
)

type Provider struct{}

func init() {
	einterfaces.RegisterOAuthProvider(model.ServiceOpenFederatedAuth, &Provider{})
}

func (p *Provider) GetSSOSettings(_ request.CTX, config *model.Config, _ string) (*model.SSOSettings, error) {
	return &config.OpenFederatedAuthSettings.SSOSettings, nil
}

func (p *Provider) GetUserFromJSON(rctx request.CTX, data io.Reader, tokenUser *model.User, settings *model.SSOSettings) (*model.User, error) {
	claims := map[string]any{}
	decoder := json.NewDecoder(data)
	decoder.UseNumber()
	if err := decoder.Decode(&claims); err != nil {
		return nil, err
	}

	return userFromClaims(rctx.Logger(), claims, tokenUser, settings)
}

func (p *Provider) GetUserFromIdToken(_ request.CTX, idToken string) (*model.User, error) {
	claims, err := claimsFromJWT(idToken)
	if err != nil {
		return nil, err
	}

	authData := strings.TrimSpace(stringClaim(claims, "sub"))
	if authData == "" {
		return nil, errors.New("id_token sub claim is empty")
	}

	return &model.User{
		Email:       strings.ToLower(strings.TrimSpace(stringClaim(claims, "email"))),
		Username:    stringClaim(claims, "preferred_username"),
		FirstName:   stringClaim(claims, "given_name"),
		LastName:    stringClaim(claims, "family_name"),
		Nickname:    stringClaim(claims, "name"),
		AuthData:    &authData,
		AuthService: model.ServiceOpenFederatedAuth,
	}, nil
}

func (p *Provider) IsSameUser(_ request.CTX, dbUser, oauthUser *model.User) bool {
	return dbUser.GetAuthData() == oauthUser.GetAuthData()
}

func userFromClaims(logger mlog.LoggerIFace, claims map[string]any, tokenUser *model.User, settings *model.SSOSettings) (*model.User, error) {
	emailClaimName := "email"
	if settings != nil && settings.EmailClaimName != nil && strings.TrimSpace(*settings.EmailClaimName) != "" {
		emailClaimName = strings.TrimSpace(*settings.EmailClaimName)
	}

	mattermostIDClaimName := "sub"
	if settings != nil && settings.MattermostIDClaimName != nil && strings.TrimSpace(*settings.MattermostIDClaimName) != "" {
		mattermostIDClaimName = strings.TrimSpace(*settings.MattermostIDClaimName)
	}

	email := strings.ToLower(strings.TrimSpace(stringClaim(claims, emailClaimName)))
	if email == "" && tokenUser != nil {
		email = strings.ToLower(strings.TrimSpace(tokenUser.Email))
	}
	if email == "" {
		return nil, errors.New("email claim is empty")
	}

	authData := strings.TrimSpace(stringClaim(claims, mattermostIDClaimName))
	if authData == "" && tokenUser != nil {
		authData = strings.TrimSpace(tokenUser.GetAuthData())
	}
	if authData == "" && mattermostIDClaimName != "sub" {
		authData = strings.TrimSpace(stringClaim(claims, "sub"))
	}
	if authData == "" {
		return nil, errors.New("auth data claim is empty")
	}
	if settings != nil && settings.UseProviderIDInAuthData != nil && *settings.UseProviderIDInAuthData && settings.OpenFederatedAuthProviderID != nil && strings.TrimSpace(*settings.OpenFederatedAuthProviderID) != "" {
		authData = strings.TrimSpace(*settings.OpenFederatedAuthProviderID) + ":" + authData
	}

	user := &model.User{
		Email:       email,
		FirstName:   stringClaim(claims, "given_name"),
		LastName:    stringClaim(claims, "family_name"),
		Nickname:    stringClaim(claims, "name"),
		AuthData:    &authData,
		AuthService: model.ServiceOpenFederatedAuth,
	}
	if user.FirstName == "" && tokenUser != nil {
		user.FirstName = tokenUser.FirstName
	}
	if user.LastName == "" && tokenUser != nil {
		user.LastName = tokenUser.LastName
	}
	if user.Nickname == "" && tokenUser != nil {
		user.Nickname = tokenUser.Nickname
	}

	username := stringClaim(claims, "preferred_username")
	if username == "" {
		username = stringClaim(claims, "nickname")
	}
	if username == "" && tokenUser != nil {
		username = tokenUser.Username
	}
	if username == "" {
		username = strings.Split(email, "@")[0]
	}
	user.Username = model.CleanUsername(logger, strings.Split(username, "@")[0])

	return user, nil
}

func stringClaim(claims map[string]any, name string) string {
	value, ok := claims[name]
	if !ok {
		return ""
	}

	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case []any:
		for _, item := range typed {
			claim := stringValue(item)
			if strings.TrimSpace(claim) != "" {
				return claim
			}
		}
		return ""
	case []string:
		for _, item := range typed {
			if strings.TrimSpace(item) != "" {
				return item
			}
		}
		return ""
	default:
		return stringValue(typed)
	}
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return ""
	}
}

func claimsFromJWT(token string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("id_token is not a JWT")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}

	claims := map[string]any{}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.UseNumber()
	if err = decoder.Decode(&claims); err != nil {
		return nil, err
	}

	return claims, nil
}
