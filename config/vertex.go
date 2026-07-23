package config

import (
	"fmt"
	"strings"
)

// VertexBaseURL builds a publisher models base URL for Vertex AI dual-path
// deployments (#136).
//
//	https://{location}-aiplatform.googleapis.com/v1/projects/{project}/locations/{location}/publishers/google
//
// Operators set this as provider.base_url (append nothing — Path() adds /models/…).
// Global endpoint: pass location "global" → https://aiplatform.googleapis.com/v1/...
func VertexBaseURL(project, location string) string {
	project = strings.TrimSpace(project)
	location = strings.TrimSpace(location)
	if project == "" {
		return ""
	}
	if location == "" {
		location = "us-central1"
	}
	host := location + "-aiplatform.googleapis.com"
	if strings.EqualFold(location, "global") {
		host = "aiplatform.googleapis.com"
	}
	return fmt.Sprintf("https://%s/v1/projects/%s/locations/%s/publishers/google",
		host, project, location)
}
