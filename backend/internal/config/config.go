package config

import (
	"os"
	"strings"
)

type Settings struct {
	Kubeconfig  string
	AWSRegion   string
	CORSOrigins []string
	LogLevel    string
}

func Load() Settings {
	return Settings{
		Kubeconfig:  getenv("KUBECONFIG", ""),
		AWSRegion:   getenv("AWS_REGION", "us-west-2"),
		CORSOrigins: splitCSV(getenv("CORS_ORIGINS", "http://localhost:5173")),
		LogLevel:    getenv("LOG_LEVEL", "INFO"),
	}
}

func getenv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
