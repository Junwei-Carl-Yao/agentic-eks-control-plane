package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("KUBECONFIG", "")
	t.Setenv("AWS_REGION", "")
	t.Setenv("CORS_ORIGINS", "")
	t.Setenv("LOG_LEVEL", "")

	s := Load()
	if s.AWSRegion != "us-west-2" {
		t.Errorf("AWSRegion = %q, want us-west-2", s.AWSRegion)
	}
	if s.LogLevel != "INFO" {
		t.Errorf("LogLevel = %q, want INFO", s.LogLevel)
	}
	if len(s.CORSOrigins) != 1 || s.CORSOrigins[0] != "http://localhost:5173" {
		t.Errorf("CORSOrigins = %v, want [http://localhost:5173]", s.CORSOrigins)
	}
}

func TestLoadEnvOverride(t *testing.T) {
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("CORS_ORIGINS", "http://a.test, http://b.test")
	t.Setenv("LOG_LEVEL", "DEBUG")

	s := Load()
	if s.AWSRegion != "us-east-1" {
		t.Errorf("AWSRegion = %q, want us-east-1", s.AWSRegion)
	}
	if s.LogLevel != "DEBUG" {
		t.Errorf("LogLevel = %q, want DEBUG", s.LogLevel)
	}
	want := []string{"http://a.test", "http://b.test"}
	if len(s.CORSOrigins) != len(want) || s.CORSOrigins[0] != want[0] || s.CORSOrigins[1] != want[1] {
		t.Errorf("CORSOrigins = %v, want %v", s.CORSOrigins, want)
	}
}
