package livelisten

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckBearerToken(t *testing.T) {
	dir := t.TempDir()
	token := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ123456789012"
	if err := os.WriteFile(filepath.Join(dir, "dispatcher"), []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	l := New(nil, Config{TokensPath: dir})
	req, err := http.NewRequest(http.MethodPost, streamPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if !l.checkBearer(req) {
		t.Fatal("expected bearer token to authenticate")
	}
	req.Header.Set("Authorization", "Bearer wrong")
	if l.checkBearer(req) {
		t.Fatal("unexpected authentication with wrong token")
	}
}
