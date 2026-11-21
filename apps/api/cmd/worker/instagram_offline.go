//go:build instagramoffline

package main

import (
	"context"
	"os"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/channel"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/instagram"
)

func instagramQueue(ctx context.Context, s instagram.Service, environment string) (channel.Queue, error) {
	outcome := os.Getenv("ABA_INSTAGRAM_OFFLINE_PROVIDER")
	if outcome == "" {
		return s, nil
	}
	return instagram.NewOfflineQueue(ctx, s, environment, outcome)
}
