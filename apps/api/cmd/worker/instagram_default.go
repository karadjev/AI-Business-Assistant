//go:build !instagramoffline

package main

import (
	"context"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/channel"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/instagram"
)

func instagramQueue(_ context.Context, s instagram.Service, _ string) (channel.Queue, error) {
	return s, nil
}
