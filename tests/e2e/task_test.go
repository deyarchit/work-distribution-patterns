package e2e_test

import (
	"os"
	"testing"

	"work-distribution-patterns/shared/testutil"
)

// baseURL returns the target API base URL (default: http://localhost:8080).
func baseURL() string {
	if u := os.Getenv("BASE_URL"); u != "" {
		return u
	}
	return "http://localhost:8080"
}

func TestE2E(t *testing.T) {
	testutil.RunSuite(t, baseURL())
}
