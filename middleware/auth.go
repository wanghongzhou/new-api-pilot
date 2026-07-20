package middleware

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"new-api-pilot/common"
	"new-api-pilot/constant"
)

var (
	ErrIdentityMissing = errors.New("platform identity is missing")
	ErrIdentityInvalid = errors.New("platform identity is invalid")
)

type Identity = common.Identity

type IdentityResolver interface {
	ResolveIdentity(*gin.Context) (Identity, error)
}

type IdentityLoader interface {
	LoadIdentity(context.Context, string) (Identity, error)
}

type SessionIdentityResolver struct {
	Store  *common.SessionStore
	Loader IdentityLoader
}

func (resolver SessionIdentityResolver) ResolveIdentity(c *gin.Context) (Identity, error) {
	if resolver.Store == nil || resolver.Loader == nil {
		return Identity{}, ErrIdentityMissing
	}
	sessionIdentity, err := resolver.Store.Read(c.Request)
	if err != nil {
		return Identity{}, ErrIdentityInvalid
	}
	currentIdentity, err := resolver.Loader.LoadIdentity(c.Request.Context(), sessionIdentity.ID)
	if err != nil {
		return Identity{}, ErrIdentityInvalid
	}
	if sessionIdentity.SessionVersion != currentIdentity.SessionVersion {
		return Identity{}, ErrIdentityInvalid
	}
	return currentIdentity, nil
}

func UserAuth(resolver IdentityResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, err := resolver.ResolveIdentity(c)
		if err != nil || identity.ID == "" || identity.Status != 1 {
			common.AbortError(c, http.StatusUnauthorized, constant.CodeAuthInvalid, "Authentication required", nil)
			return
		}
		if c.GetHeader("New-Api-User") != identity.ID {
			common.AbortError(c, http.StatusUnauthorized, constant.CodeAuthInvalid, "Authentication required", nil)
			return
		}
		if identity.Role != constant.RoleAdmin && identity.Role != constant.RoleViewer {
			common.AbortError(c, http.StatusUnauthorized, constant.CodeAuthInvalid, "Authentication required", nil)
			return
		}
		c.Set(constant.ContextIdentity, identity)
		c.Next()
	}
}

func AdminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, ok := CurrentIdentity(c)
		if !ok {
			common.AbortError(c, http.StatusUnauthorized, constant.CodeAuthRequired, "Authentication required", nil)
			return
		}
		if identity.Role != constant.RoleAdmin {
			common.AbortError(c, http.StatusForbidden, constant.CodeForbidden, "Insufficient permissions", nil)
			return
		}
		c.Next()
	}
}

func ForcePasswordChange() gin.HandlerFunc {
	allowed := map[string]struct{}{
		http.MethodGet + " /api/user/self":     {},
		http.MethodPut + " /api/user/password": {},
		http.MethodPost + " /api/user/logout":  {},
	}
	return func(c *gin.Context) {
		identity, ok := CurrentIdentity(c)
		if !ok || !identity.MustChangePassword {
			c.Next()
			return
		}
		if _, exists := allowed[c.Request.Method+" "+c.Request.URL.Path]; !exists {
			common.AbortError(c, http.StatusForbidden, constant.CodePasswordChangeRequired, "Password change required", nil)
			return
		}
		c.Next()
	}
}

func CurrentIdentity(c *gin.Context) (Identity, bool) {
	value, exists := c.Get(constant.ContextIdentity)
	if !exists {
		return Identity{}, false
	}
	identity, ok := value.(Identity)
	return identity, ok
}
