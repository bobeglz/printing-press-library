package cli

import (
	"context"
	"fmt"
	"github.com/mvanhorn/printing-press-library/library/developer-tools/crocoblock/internal/client"
	"github.com/mvanhorn/printing-press-library/library/developer-tools/crocoblock/internal/store"
	"os"
	"path/filepath"
)

func crocoDBPath() string {
	if v := os.Getenv("CROCOBLOCK_DB"); v != "" {
		return v
	}
	return defaultDBPath("crocoblock-pp-cli")
}
func crocoOpenStore(ctx context.Context, path string) (*store.Store, error) {
	if path == "" {
		path = crocoDBPath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	s, e := store.OpenWithContext(ctx, path)
	if e != nil {
		return nil, e
	}
	if e = os.Chmod(path, 0600); e != nil {
		s.Close()
		return nil, e
	}
	return s, nil
}
func crocoJournal(f *rootFlags, ctx context.Context, c *client.Client, kind, resource string, fields []string, detail map[string]any) error {
	s, e := crocoOpenStore(ctx, "")
	if e != nil {
		return apiErr(fmt.Errorf("mutation_applied: local journal unavailable; do not retry write: %w", e))
	}
	defer s.Close()
	_, e = s.CrocoEvent(ctx, c.BaseURL, kind, resource, fields, detail)
	if e != nil {
		return apiErr(fmt.Errorf("mutation_applied: journal failed; verify upstream before any retry: %w", e))
	}
	return nil
}
