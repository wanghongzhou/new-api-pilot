package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	acceptanceID       = "A22"
	fixtureID          = "F05"
	defaultFixturePath = "testdata/design/f05-ops-capacity.yaml"
	databaseName       = "pilot_a22"
)

type a22Profile struct {
	SchemaVersion int    `yaml:"schema_version"`
	FixtureID     string `yaml:"fixture_id"`
	Clock         struct {
		Timezone string `yaml:"timezone"`
		Now      string `yaml:"now"`
		NowUnix  int64  `yaml:"now_unix"`
	} `yaml:"clock"`
	MySQL struct {
		Version              string `yaml:"version"`
		TransactionIsolation string `yaml:"transaction_isolation"`
		Charset              string `yaml:"charset"`
		Collation            string `yaml:"collation"`
		BinlogEnabled        bool   `yaml:"binlog_enabled"`
		RPOSeconds           int64  `yaml:"rpo_seconds"`
		RTOSeconds           int64  `yaml:"rto_seconds"`
		TamperMustFail       bool   `yaml:"migration_checksum_tamper_must_fail"`
	} `yaml:"mysql"`
}

func loadProfile(path string) (a22Profile, string, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return a22Profile{}, "", err
	}
	var profile a22Profile
	if err := yaml.Unmarshal(payload, &profile); err != nil {
		return a22Profile{}, "", err
	}
	parsed, err := time.Parse(time.RFC3339, profile.Clock.Now)
	if err != nil || parsed.Unix() != profile.Clock.NowUnix {
		return a22Profile{}, "", errors.New("F05 clock contract is invalid")
	}
	if profile.SchemaVersion != 2 || profile.FixtureID != fixtureID || profile.Clock.Timezone != "Asia/Shanghai" ||
		profile.MySQL.Version != "8.4" || profile.MySQL.TransactionIsolation != "READ-COMMITTED" ||
		profile.MySQL.Charset != "utf8mb4" || profile.MySQL.Collation != "utf8mb4_unicode_ci" ||
		!profile.MySQL.BinlogEnabled || !profile.MySQL.TamperMustFail ||
		profile.MySQL.RPOSeconds != 3600 || profile.MySQL.RTOSeconds != 14400 {
		return a22Profile{}, "", errors.New("F05 A22 profile contract is invalid")
	}
	digest := sha256.Sum256(payload)
	return profile, hex.EncodeToString(digest[:]), nil
}

func fixtureHour(profile a22Profile) int64 {
	return profile.Clock.NowUnix - profile.Clock.NowUnix%3600
}

func fixtureDateKey(profile a22Profile) int {
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	value := time.Unix(profile.Clock.NowUnix, 0).In(location)
	return value.Year()*10000 + int(value.Month())*100 + value.Day()
}
