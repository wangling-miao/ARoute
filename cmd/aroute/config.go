// Package main implements the config subcommand for ARoute CMS.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
	Long: `View and validate ARoute CMS configuration.

Configuration precedence (highest to lowest):
  1. CLI flags (e.g., --port)
  2. Environment variables (AROUTE_* prefix)
  3. Config file (aroute.yaml, aroute.toml)
  4. Built-in defaults`,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Long:  `Display the fully resolved configuration after applying all precedence rules.`,
	Run:   runConfigShow,
}

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate configuration file",
	Long:  `Validate the configuration file for syntax and semantic errors.`,
	Run:   runConfigValidate,
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configValidateCmd)
}

func runConfigShow(cmd *cobra.Command, args []string) {
	// Get all settings as map
	settings := viper.AllSettings()

	// Mask sensitive values
	maskSensitive(settings)

	// Output as YAML
	data, err := yaml.Marshal(settings)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(data))
}

func runConfigValidate(cmd *cobra.Command, args []string) {
	// Check if config file was found
	configFile := viper.ConfigFileUsed()
	if configFile == "" {
		fmt.Println("No config file specified. Using defaults and environment variables only.")
		fmt.Println("Configuration is valid.")
		return
	}

	// Re-read the config file to check for errors
	if err := viper.ReadInConfig(); err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	// Check for unknown keys (compare against known defaults)
	knownKeys := getKnownKeys()
	settings := viper.AllSettings()
	unknownKeys := findUnknownKeys(settings, knownKeys)

	if len(unknownKeys) > 0 {
		for _, key := range unknownKeys {
			suggestion := findSuggestion(key, knownKeys)
			if suggestion != "" {
				fmt.Printf("WARNING: unknown config key \"%s\" — did you mean \"%s\"?\n", key, suggestion)
			} else {
				fmt.Printf("WARNING: unknown config key \"%s\"\n", key)
			}
		}
	}

	fmt.Println("Configuration is valid.")
	fmt.Printf("Config file: %s\n", configFile)
}

// maskSensitive masks sensitive configuration values
func maskSensitive(settings map[string]interface{}) {
	sensitiveKeys := []string{
		"jwt_secret",
		"secret",
		"password",
		"access_key",
		"secret_key",
		"api_key",
	}

	for key, value := range settings {
		if nested, ok := value.(map[string]interface{}); ok {
			maskSensitive(nested)
		} else {
			for _, sensitive := range sensitiveKeys {
				if strings.Contains(strings.ToLower(key), sensitive) {
					settings[key] = "****"
					break
				}
			}
		}
	}
}

// getKnownKeys returns all known configuration keys
func getKnownKeys() []string {
	return []string{
		"server.host",
		"server.port",
		"database.driver",
		"database.sqlite.path",
		"database.postgres.host",
		"database.postgres.port",
		"database.postgres.user",
		"database.postgres.password",
		"database.postgres.dbname",
		"database.postgres.sslmode",
		"auth.jwt_secret",
		"auth.jwt_algorithm",
		"auth.jwt_private_key_path",
		"auth.jwt_public_key_path",
		"auth.access_token_ttl",
		"auth.refresh_token_ttl",
		"media.storage",
		"media.local.upload_dir",
		"media.s3.endpoint",
		"media.s3.bucket",
		"media.s3.region",
		"media.s3.access_key",
		"media.s3.secret_key",
		"media.s3.use_ssl",
		"media.max_file_size",
		"media.allowed_types",
		"search.index_dir",
		"cache.max_size",
		"cache.default_ttl",
		"theme.active",
		"theme.dir",
		"log.level",
		"log.format",
		"cors.allowed_origins",
		"cors.allowed_methods",
		"cors.allowed_headers",
		"plugins.dir",
		"data_dir",
	}
}

// findUnknownKeys finds keys that are not in the known list
func findUnknownKeys(settings map[string]interface{}, knownKeys []string) []string {
	var unknown []string
	flattenKeys(settings, "", &unknown)

	// Filter out known keys
	result := []string{}
	for _, key := range unknown {
		found := false
		for _, known := range knownKeys {
			if key == known {
				found = true
				break
			}
		}
		if !found {
			result = append(result, key)
		}
	}
	return result
}

// flattenKeys flattens nested map keys
func flattenKeys(m map[string]interface{}, prefix string, keys *[]string) {
	for key, value := range m {
		fullKey := prefix + key
		if nested, ok := value.(map[string]interface{}); ok {
			flattenKeys(nested, fullKey+".", keys)
		} else {
			*keys = append(*keys, fullKey)
		}
	}
}

// findSuggestion finds a similar key for typo detection
func findSuggestion(unknown string, knownKeys []string) string {
	for _, known := range knownKeys {
		// Simple similarity check
		if similar(unknown, known) {
			return known
		}
	}
	return ""
}

// similar checks if two strings are similar (typo detection)
func similar(a, b string) bool {
	if len(a) == len(b) {
		diff := 0
		for i := range a {
			if a[i] != b[i] {
				diff++
				if diff > 2 {
					return false
				}
			}
		}
		return diff <= 2
	}
	if len(a) == len(b)+1 || len(a)+1 == len(b) {
		shorter, longer := a, b
		if len(a) > len(b) {
			shorter, longer = b, a
		}
		diff := 0
		for i, j := 0, 0; i < len(shorter) && j < len(longer); {
			if shorter[i] == longer[j] {
				i++
				j++
			} else {
				diff++
				j++
				if diff > 1 {
					return false
				}
			}
		}
		return true
	}
	return false
}
