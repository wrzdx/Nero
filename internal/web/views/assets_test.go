package web_views

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLayoutAssetVersionsMatchShippedFiles(t *testing.T) {
	var page bytes.Buffer
	if err := Layout("Nero").Render(context.Background(), &page); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"css/app.css", "js/auth.js", "js/app.js", "js/participants.js"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "..", "web", "static", filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(data)
		url := fmt.Sprintf("/static/%s?v=%x", path, hash[:8])
		if !strings.Contains(page.String(), `"`+url+`"`) {
			t.Errorf("layout must load the current %s by content hash; run npm run build:ui", path)
		}
	}
}
