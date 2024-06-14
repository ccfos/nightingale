package osx

import "os"

// getEnv returns the value of an environment variable, or returns the provided fallback value
func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
