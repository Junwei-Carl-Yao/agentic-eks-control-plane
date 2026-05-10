package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("KUBECONFIG", "")
	t.Setenv("AWS_REGION", "")
	t.Setenv("CORS_ORIGINS", "")

	settings := Load()
	if settings.AWSRegion != "us-east-1" {
		t.Errorf("AWSRegion = %q, want us-east-1", settings.AWSRegion)
	}
	if len(settings.CORSOrigins) != 1 || settings.CORSOrigins[0] != "http://localhost:5173" {
		t.Errorf("CORSOrigins = %v, want [http://localhost:5173]", settings.CORSOrigins)
	}
}

func TestLoadEnvOverride(t *testing.T) {
	t.Setenv("AWS_REGION", "us-west-2")
	t.Setenv("CORS_ORIGINS", "http://a.test, http://b.test")

	settings := Load()
	if settings.AWSRegion != "us-west-2" {
		t.Errorf("AWSRegion = %q, want us-west-2", settings.AWSRegion)
	}
	expectedOrigins := []string{"http://a.test", "http://b.test"}
	if len(settings.CORSOrigins) != len(expectedOrigins) || settings.CORSOrigins[0] != expectedOrigins[0] || settings.CORSOrigins[1] != expectedOrigins[1] {
		t.Errorf("CORSOrigins = %v, want %v", settings.CORSOrigins, expectedOrigins)
	}
}
