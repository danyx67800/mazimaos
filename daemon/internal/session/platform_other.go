//go:build !linux

package session

import "context"

func startPlatformOS(ctx context.Context, s *Session, dataDir string, cmd Command) (platform, string, error) {
	return nil, "", ErrUnsupported
}

var _ = context.Background
