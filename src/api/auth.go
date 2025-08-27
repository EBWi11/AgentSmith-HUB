package api

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/labstack/echo/v4"
)

// Public response for frontend to decide login flow
type AuthConfigResponse struct {
	OIDCEnabled   bool   `json:"oidc_enabled"`
	Issuer        string `json:"issuer,omitempty"`
	ClientID      string `json:"client_id,omitempty"`
	UsernameClaim string `json:"username_claim,omitempty"`
	RedirectURI   string `json:"redirect_uri,omitempty"`
	Scope         string `json:"scope,omitempty"`
}

var (
	oidcOnce      sync.Once
	oidcInitErr   error
	oidcProvider  *oidc.Provider
	idTokenVerify *oidc.IDTokenVerifier
)

func ensureOIDCVerifier(ctx context.Context) error {
	oidcOnce.Do(func() {
		if !common.Config.OIDCEnabled {
			oidcInitErr = nil
			return
		}
		if common.Config.OIDCIssuer == "" || common.Config.OIDCClientID == "" {
			oidcInitErr = errors.New("OIDC issuer or client_id not configured")
			return
		}
		provider, err := oidc.NewProvider(ctx, common.Config.OIDCIssuer)
		if err != nil {
			oidcInitErr = err
			return
		}
		oidcProvider = provider
		verifier := provider.Verifier(&oidc.Config{ClientID: common.Config.OIDCClientID})
		idTokenVerify = verifier
	})
	return oidcInitErr
}

// VerifyOIDCToken verifies an ID token and returns claims map
func VerifyOIDCToken(ctx context.Context, rawToken string) (map[string]interface{}, error) {
	if !common.Config.OIDCEnabled {
		return nil, errors.New("OIDC not enabled")
	}
	if err := ensureOIDCVerifier(ctx); err != nil {
		return nil, err
	}
	if idTokenVerify == nil {
		return nil, errors.New("OIDC verifier not initialized")
	}
	idTok, err := idTokenVerify.Verify(ctx, rawToken)
	if err != nil {
		return nil, err
	}
	var claims map[string]interface{}
	if err := idTok.Claims(&claims); err != nil {
		return nil, err
	}
	return claims, nil
}

func isUserAllowed(claims map[string]interface{}) bool {
	allowed := common.Config.OIDCAllowedUsers
	if len(allowed) == 0 {
		return false
	}
	claimKey := common.Config.OIDCUsernameClaim
	if claimKey == "" {
		// default fallbacks
		if _, ok := claims["preferred_username"]; ok {
			claimKey = "preferred_username"
		} else {
			claimKey = "email"
		}
	}
	val, _ := claims[claimKey].(string)
	if val == "" {
		return false
	}
	for _, u := range allowed {
		if strings.EqualFold(strings.TrimSpace(u), val) {
			return true
		}
	}
	return false
}

// AuthenticateRequest allows either legacy token header or OIDC Bearer token
func AuthenticateRequest(c echo.Context) error {
	// Legacy token header
	token := c.Request().Header.Get("token")
	if token != "" && token == common.Config.Token {
		return nil
	}

	// OIDC Bearer token
	authz := c.Request().Header.Get("Authorization")
	if authz == "" {
		return errors.New("empty authorization header")
	}
	if !strings.HasPrefix(strings.ToLower(authz), "bearer ") {
		return errors.New("invalid authorization header")
	}
	raw := strings.TrimSpace(authz[len("Bearer "):])
	if raw == "" {
		return errors.New("empty bearer token")
	}

	if !common.Config.OIDCEnabled {
		return errors.New("oidc not enabled")
	}

	claims, err := VerifyOIDCToken(c.Request().Context(), raw)
	if err != nil {
		logger.Warn("OIDC token verification failed", "error", err)
		return errors.New("authentication failed")
	}
	if !isUserAllowed(claims) {
		return errors.New("user not allowed")
	}
	return nil
}

// GET /auth/config
func getAuthConfig(c echo.Context) error {
	resp := AuthConfigResponse{
		OIDCEnabled:   common.Config.OIDCEnabled,
		Issuer:        common.Config.OIDCIssuer,
		ClientID:      common.Config.OIDCClientID,
		UsernameClaim: common.Config.OIDCUsernameClaim,
		RedirectURI:   common.Config.OIDCRedirectURI,
		Scope:         common.Config.OIDCScope,
	}
	return c.JSON(http.StatusOK, resp)
}
