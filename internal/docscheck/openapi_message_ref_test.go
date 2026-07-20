package docscheck

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestMessageRefOpenAPIIsDeterministic(t *testing.T) {
	first := &messageCatalog{APIErrorCodes: []string{"HTTP_ONE"}, Entries: []messageCatalogEntry{
		{Code: "MESSAGE_TWO", Category: "alert", RequiredParams: map[string]string{"site_id": "IdString", "value": "number"}, OptionalParams: map[string]string{}},
		{Code: "HTTP_ONE", Category: "http", RequiredParams: map[string]string{}, OptionalParams: map[string]string{}},
		{Code: "MESSAGE_ONE", Category: "data", RequiredParams: map[string]string{"scope_id": "IdString|null"}, OptionalParams: map[string]string{"detail": "string"}},
	}}
	second := &messageCatalog{APIErrorCodes: []string{"HTTP_ONE"}, Entries: []messageCatalogEntry{
		first.Entries[2],
		first.Entries[0],
		first.Entries[1],
	}}
	firstPayload, err := messageRefOpenAPIPayload(first)
	if err != nil {
		t.Fatal(err)
	}
	secondPayload, err := messageRefOpenAPIPayload(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstPayload, secondPayload) {
		t.Fatal("MessageRef OpenAPI generation depends on catalog entry order")
	}
}

func TestMessageRefOpenAPIDetectsContractAndCatalogDrift(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, filepath.FromSlash(messageRefOpenAPIPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	catalog := &messageCatalog{APIErrorCodes: []string{"HTTP_ONE"}, Entries: []messageCatalogEntry{
		{Code: "HTTP_ONE", Category: "http", RequiredParams: map[string]string{}, OptionalParams: map[string]string{}},
		{Code: "MESSAGE_ONE", Category: "alert", RequiredParams: map[string]string{"site_id": "IdString"}, OptionalParams: map[string]string{}},
	}}
	payload, err := messageRefOpenAPIPayload(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}

	current := &checker{root: root}
	current.checkMessageRefOpenAPI(catalog)
	if len(current.issues) != 0 {
		t.Fatalf("generated OpenAPI produced issues: %#v", current.issues)
	}

	if err := os.WriteFile(path, append(payload, ' '), 0o644); err != nil {
		t.Fatal(err)
	}
	current = &checker{root: root}
	current.checkMessageRefOpenAPI(catalog)
	if len(current.issues) != 1 || current.issues[0].Check != "openapi" {
		t.Fatalf("contract drift was not rejected: %#v", current.issues)
	}

	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	driftedCatalog := &messageCatalog{APIErrorCodes: []string{"HTTP_ONE"}, Entries: []messageCatalogEntry{
		catalog.Entries[0],
		{Code: "MESSAGE_ONE", Category: "alert", RequiredParams: map[string]string{"site_id": "IdString", "run_id": "IdString"}, OptionalParams: map[string]string{}},
	}}
	current = &checker{root: root}
	current.checkMessageRefOpenAPI(driftedCatalog)
	if len(current.issues) != 1 || current.issues[0].Check != "openapi" {
		t.Fatalf("catalog drift was not rejected: %#v", current.issues)
	}
}

func TestWriteMessageRefOpenAPIProducesCheckedInContract(t *testing.T) {
	root := t.TempDir()
	catalogPath := filepath.Join(root, filepath.FromSlash(messageCatalogPath))
	if err := os.MkdirAll(filepath.Dir(catalogPath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, catalogPath, `{
  "schema_version": 1,
  "source": "docs/example.md#message-ref",
  "api_error_codes": ["HTTP_ONE"],
  "entries": [
    {"code":"HTTP_ONE","category":"http","required_params":{},"optional_params":{}},
    {"code":"MESSAGE_ONE","category":"alert","required_params":{"site_id":"IdString"},"optional_params":{}}
  ]
}`)
	if err := WriteMessageRefOpenAPI(root); err != nil {
		t.Fatal(err)
	}
	current := &checker{root: root}
	catalog, err := readMessageCatalog(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	current.checkMessageRefOpenAPI(catalog)
	if len(current.issues) != 0 {
		t.Fatalf("generated contract produced issues: %#v", current.issues)
	}
}

func TestContractGenerationSynchronizesFixtureChecksumManifest(t *testing.T) {
	root := t.TempDir()
	catalogPath := filepath.Join(root, filepath.FromSlash(messageCatalogPath))
	if err := os.MkdirAll(filepath.Dir(catalogPath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, catalogPath, `{
  "schema_version": 1,
  "source": "docs/example.md#message-ref",
  "api_error_codes": ["HTTP_ONE"],
  "entries": [
    {"code":"HTTP_ONE","category":"http","required_params":{},"optional_params":{}},
    {"code":"MESSAGE_ONE","category":"alert","required_params":{"site_id":"IdString"},"optional_params":{}}
  ]
}`)
	if err := WriteMessageRefOpenAPI(root); err != nil {
		t.Fatal(err)
	}
	if err := WriteFixtureChecksumManifest(root); err != nil {
		t.Fatal(err)
	}

	catalog, err := readMessageCatalog(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	current := &checker{root: root}
	current.checkMessageRefOpenAPI(catalog)
	current.checkFixtureChecksums(&acceptanceManifest{
		Baseline: acceptanceBaseline{FixtureChecksumManifest: fixtureChecksumManifestPath},
	})
	if len(current.issues) != 0 {
		t.Fatalf("contract generation left drift behind: %#v", current.issues)
	}
}
