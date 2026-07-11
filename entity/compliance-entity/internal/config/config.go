// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

// Package config loads runtime configuration from environment variables.
package config

import (
	"fmt"
	"os"
)

// Config holds all environment-driven settings for the compliance entity service.
type Config struct {
	// DB_DSN is the full MySQL connection string.
	// Format: user:password@tcp(host:port)/dbname?parseTime=true
	DBDSN      string
	ServerPort string

	// Azure Blob Storage — the Compliance Entity is the only component that holds
	// the account key and talks to Azure (evidence/risk file bytes flow through here).
	AzureAccountName   string
	AzureAccountKey    string
	AzureContainerName string
}

// Load reads configuration from environment variables and returns a populated Config.
func Load() *Config {
	return &Config{
		DBDSN:              os.Getenv("DB_DSN"),
		ServerPort:         getEnvOrDefault("SERVER_PORT", "8080"),
		AzureAccountName:   os.Getenv("AZURE_STORAGE_ACCOUNT_NAME"),
		AzureAccountKey:    os.Getenv("AZURE_STORAGE_ACCOUNT_KEY"),
		AzureContainerName: getEnvOrDefault("AZURE_STORAGE_CONTAINER", "grc-evidence"),
	}
}

// AzureConfigured reports whether Azure Blob credentials are present. When false,
// the file (byte-storage) endpoints are disabled — useful for local dev/tests that
// only exercise the MySQL metadata endpoints.
func (c *Config) AzureConfigured() bool {
	return c.AzureAccountName != "" && c.AzureAccountKey != ""
}

func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// Validate returns an error if any required configuration is missing.
func (c *Config) Validate() error {
	if c.DBDSN == "" {
		return fmt.Errorf("DB_DSN is required")
	}
	return nil
}
