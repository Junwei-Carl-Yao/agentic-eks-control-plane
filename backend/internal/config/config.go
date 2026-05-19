package config

import (
	"os"
	"strings"
)

type Settings struct {
	Kubeconfig  string
	AWSRegion   string
	ClusterName string
	CORSOrigins []string
}

func Load() Settings {
	return Settings{
		Kubeconfig:  getenv("KUBECONFIG", ""),
		AWSRegion:   getenv("AWS_REGION", "us-east-1"),
		ClusterName: getenv("CLUSTER_NAME", "eks-cluster"),
		CORSOrigins: splitCSV(getenv("CORS_ORIGINS", "http://localhost:5173")),
	}
}

func getenv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	output := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			output = append(output, trimmed)
		}
	}
	return output
}
