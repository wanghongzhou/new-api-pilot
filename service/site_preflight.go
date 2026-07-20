package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"new-api-pilot/common"
	"new-api-pilot/dto"
	"new-api-pilot/model"
)

type sitePreflightClaims struct {
	SiteID        int64  `json:"site_id"`
	ConfigVersion int    `json:"config_version"`
	BaseURL       string `json:"base_url"`
	ExpiresAt     int64  `json:"expires_at"`
}

func (service *SiteService) PreflightBaseURL(ctx context.Context, siteID int64, candidate, requestID string) (dto.SiteBaseURLPreflightResult, error) {
	site, err := service.sites.FindByID(ctx, siteID)
	if err != nil {
		if model.IsNotFound(err) {
			return dto.SiteBaseURLPreflightResult{}, ErrSiteNotFound
		}
		return dto.SiteBaseURLPreflightResult{}, err
	}
	normalized, err := NormalizeUpstreamBaseURL(candidate)
	if err != nil {
		return dto.SiteBaseURLPreflightResult{}, ErrSiteInvalidBaseURL
	}
	client, err := service.clients.NewPublic(normalized)
	if err != nil {
		return dto.SiteBaseURLPreflightResult{}, err
	}
	defer client.CloseIdleConnections()
	status, statusErr := client.Status(ctx, requestID)
	contractStatus := "compatible"
	if statusErr != nil {
		switch {
		case errors.Is(statusErr, ErrUpstreamExportDisabled):
			// Export availability is shown during authorization; the public DTO is compatible.
		default:
			return dto.SiteBaseURLPreflightResult{}, statusErr
		}
	}
	expiresAt := service.clock.Now().Add(sitePreflightLifetime).Unix()
	token, err := service.signPreflightToken(sitePreflightClaims{
		SiteID: site.ID, ConfigVersion: site.ConfigVersion, BaseURL: normalized, ExpiresAt: expiresAt,
	})
	if err != nil {
		return dto.SiteBaseURLPreflightResult{}, err
	}
	return dto.SiteBaseURLPreflightResult{
		NormalizedBaseURL: normalized, ChangeType: compareSiteBaseURLs(site.BaseURL, normalized),
		OldPublic:       dto.SitePublicIdentity{BaseURL: site.BaseURL, SystemName: site.SystemName, Version: site.Version},
		CandidatePublic: dto.SitePublicIdentity{BaseURL: normalized, SystemName: status.SystemName, Version: status.Version},
		ContractStatus:  contractStatus,
		PreflightToken:  token, ExpiresAt: expiresAt,
	}, nil
}

func (service *SiteService) signPreflightToken(claims sitePreflightClaims) (string, error) {
	payload, err := common.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("encode site preflight token: %w", err)
	}
	mac := hmac.New(sha256.New, service.preflightSecret)
	_, _ = mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (service *SiteService) verifyPreflightToken(token string) (sitePreflightClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return sitePreflightClaims{}, ErrBaseURLPreflightRequired
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return sitePreflightClaims{}, ErrBaseURLPreflightRequired
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return sitePreflightClaims{}, ErrBaseURLPreflightRequired
	}
	mac := hmac.New(sha256.New, service.preflightSecret)
	_, _ = mac.Write(payload)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return sitePreflightClaims{}, ErrBaseURLPreflightRequired
	}
	var claims sitePreflightClaims
	if err := common.Unmarshal(payload, &claims); err != nil || claims.SiteID <= 0 || claims.ConfigVersion <= 0 ||
		claims.BaseURL == "" || claims.ExpiresAt <= service.clock.Now().Unix() {
		return sitePreflightClaims{}, ErrBaseURLPreflightRequired
	}
	return claims, nil
}

func compareSiteBaseURLs(current, candidate string) string {
	if current == candidate {
		return "none"
	}
	currentURL, currentErr := url.Parse(current)
	candidateURL, candidateErr := url.Parse(candidate)
	if currentErr != nil || candidateErr != nil {
		return "origin"
	}
	if normalizedOrigin(currentURL) == normalizedOrigin(candidateURL) {
		return "path"
	}
	return "origin"
}

func normalizedOrigin(value *url.URL) string {
	if value == nil {
		return ""
	}
	return strings.ToLower(value.Scheme) + "://" + strings.ToLower(value.Host)
}
