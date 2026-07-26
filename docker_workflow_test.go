package main

import (
	"os"
	"strings"
	"testing"
)

func TestDockerWorkflowPublishesLatestFromMainAndTags(t *testing.T) {
	raw, err := os.ReadFile(".github/workflows/docker.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(raw)

	if !strings.Contains(workflow, "type=raw,value=latest,enable=${{ github.ref == 'refs/heads/main' || startsWith(github.ref, 'refs/tags/') }}") {
		t.Fatal("Docker workflow must publish latest for main and version tags")
	}
	if strings.Contains(workflow, "type=raw,value=latest,enable=${{ startsWith(github.ref, 'refs/tags/') }}") {
		t.Fatal("Docker workflow must not limit latest to version tags")
	}
	if !strings.Contains(workflow, "version=$(sed -n") || !strings.Contains(workflow, "utils/version.go)") {
		t.Fatal("main Docker builds must use the repository AppVersion")
	}
}
