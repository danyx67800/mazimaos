package session

import "context"

type platform interface {
	Input(ctx context.Context, ev InputEvent) error
	ClipboardGet(ctx context.Context) (string, error)
	ClipboardSet(ctx context.Context, text string) error
	Close()
}

func startPlatform(ctx context.Context, s *Session, dataDir string, cmd Command) (platform, string, error) {
	return startPlatformOS(ctx, s, dataDir, cmd)
}
