package docscheck

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFrontendMessageContractMatchesCatalog(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, filepath.FromSlash(frontendMessageCodesPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, path, `export const API_ERROR_CODES = ['HTTP_ONE'] as const
export const MESSAGE_CODES = ['MESSAGE_ONE'] as const
`)
	catalog := &messageCatalog{APIErrorCodes: []string{"HTTP_ONE"}, Entries: []messageCatalogEntry{
		{Code: "HTTP_ONE", Category: "http", RequiredParams: map[string]string{}, OptionalParams: map[string]string{}},
		{Code: "MESSAGE_ONE", Category: "alert", RequiredParams: map[string]string{}, OptionalParams: map[string]string{}},
	}}
	current := &checker{root: root}
	current.checkFrontendMessageContract(catalog)
	if len(current.issues) != 0 {
		t.Fatalf("matching frontend contract produced issues: %#v", current.issues)
	}

	writeTestFile(t, path, `export const API_ERROR_CODES = [] as const
export const MESSAGE_CODES = ['MESSAGE_ONE', 'EXTRA'] as const
`)
	current = &checker{root: root}
	current.checkFrontendMessageContract(catalog)
	if len(current.issues) != 2 {
		t.Fatalf("frontend drift produced issues=%#v, want two", current.issues)
	}
}
