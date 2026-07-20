package common

import "testing"

func TestPasswordValidationUsesRunesAndUTF8Bytes(t *testing.T) {
	if err := ValidatePassword("短密码七个字"); err == nil {
		t.Fatal("ValidatePassword() accepted fewer than 8 runes")
	}
	if err := ValidatePassword("八个汉字刚刚超过限制长度测试"); err != nil {
		t.Fatalf("valid Unicode password rejected: %v", err)
	}
	if err := ValidatePassword("界界界界界界界界界界界界界界界界界界界界界界界界界"); err == nil {
		t.Fatal("ValidatePassword() accepted more than 72 UTF-8 bytes")
	}
}

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := HashPassword("change-me")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if err := CheckPassword(hash, "change-me"); err != nil {
		t.Fatalf("CheckPassword() error = %v", err)
	}
	if err := CheckPassword(hash, "wrong-pass"); err == nil {
		t.Fatal("CheckPassword() accepted the wrong password")
	}
}
