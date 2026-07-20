package model

import "testing"

func TestValidateMySQLVersion(t *testing.T) {
	for _, version := range []string{"8.0.36", "8.4.6", "9.0.1-commercial"} {
		if err := ValidateMySQLVersion(version); err != nil {
			t.Fatalf("ValidateMySQLVersion(%q) error = %v", version, err)
		}
	}
	for _, version := range []string{"5.7.44", "10.11.4-MariaDB", "unknown"} {
		if err := ValidateMySQLVersion(version); err == nil {
			t.Fatalf("ValidateMySQLVersion(%q) succeeded", version)
		}
	}
}

func TestValidateMySQLCharsetAndCollation(t *testing.T) {
	if err := validateMySQLCharsetAndCollation("utf8mb4", "utf8mb4_unicode_ci"); err != nil {
		t.Fatalf("valid charset/collation error = %v", err)
	}
	for _, test := range []struct {
		charset   string
		collation string
	}{
		{charset: "utf8mb4", collation: "utf8mb4_0900_ai_ci"},
		{charset: "utf8mb4", collation: "utf8mb4_bin"},
		{charset: "utf8", collation: "utf8mb4_unicode_ci"},
	} {
		if err := validateMySQLCharsetAndCollation(test.charset, test.collation); err == nil {
			t.Errorf("validateMySQLCharsetAndCollation(%q, %q) succeeded", test.charset, test.collation)
		}
	}
}
