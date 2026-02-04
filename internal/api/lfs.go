package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/google/go-github/v62/github"
	"github.com/shurcooL/githubv4"
)

// LFSObject represents a Git LFS object with its OID and size
type LFSObject struct {
	OID  string `json:"oid"`
	Size int64  `json:"size"`
}

// LFSBatchRequest represents the request body for the LFS batch API
type LFSBatchRequest struct {
	Operation string      `json:"operation"`
	Transfers []string    `json:"transfers"`
	Objects   []LFSObject `json:"objects"`
}

// LFSBatchResponse represents the response from the LFS batch API
type LFSBatchResponse struct {
	Objects []LFSBatchObject `json:"objects"`
}

// LFSBatchObject represents a single object in the batch response
type LFSBatchObject struct {
	OID     string               `json:"oid"`
	Size    int64                `json:"size"`
	Error   *LFSBatchObjectError `json:"error,omitempty"`
	Actions map[string]LFSAction `json:"actions,omitempty"`
}

// LFSBatchObjectError represents an error for a specific object
type LFSBatchObjectError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// LFSAction represents an action (upload/download) for an LFS object
type LFSAction struct {
	Href string `json:"href"`
}

// GetLFSObjects retrieves all LFS objects (OIDs) referenced in the repository
// by first reading .gitattributes to find LFS-tracked patterns, then only
// checking files that match those patterns for LFS pointer content
func (api *GitHubAPI) GetLFSObjects(clientType ClientType, owner, name string) ([]LFSObject, error) {
	ctx := context.Background()

	// First, get the default branch to know which ref to query
	var repoQuery struct {
		Repository struct {
			DefaultBranchRef struct {
				Name string
			}
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner": githubv4.String(owner),
		"name":  githubv4.String(name),
	}

	client, clientName, err := api.getGraphQLClient(clientType)
	if err != nil {
		return nil, err
	}

	err = client.Query(ctx, &repoQuery, variables)
	if err != nil {
		return nil, fmt.Errorf("failed to query %s repository default branch: %v", clientName, err)
	}

	defaultBranch := repoQuery.Repository.DefaultBranchRef.Name
	if defaultBranch == "" {
		// Repository might be empty or have no default branch
		return []LFSObject{}, nil
	}

	// Use REST API to get LFS patterns and files
	restClient, _, err := api.getRESTClient(clientType)
	if err != nil {
		return nil, err
	}

	// Get LFS-tracked patterns from .gitattributes
	lfsPatterns, err := api.getLFSPatternsFromGitAttributes(ctx, restClient, owner, name, defaultBranch)
	if err != nil {
		// If we can't read .gitattributes, fall back to checking all small files
		// This is not an error - the repo might not use LFS or might not have .gitattributes
		return api.getLFSObjectsFallback(ctx, restClient, owner, name, defaultBranch, clientName)
	}

	// If no LFS patterns found, return empty list
	if len(lfsPatterns) == 0 {
		return []LFSObject{}, nil
	}

	lfsObjects := make([]LFSObject, 0)
	seenOIDs := make(map[string]bool) // Track OIDs to deduplicate

	// Get the repository tree recursively
	tree, _, err := restClient.Git.GetTree(ctx, owner, name, defaultBranch, true)
	if err != nil {
		return nil, fmt.Errorf("failed to get %s repository tree: %v", clientName, err)
	}

	// Process each file in the tree
	for _, entry := range tree.Entries {
		// Only process blob entries (files)
		if entry.GetType() != "blob" {
			continue
		}

		// Check if this file matches any LFS pattern
		filePath := entry.GetPath()
		if !matchesLFSPattern(filePath, lfsPatterns) {
			continue
		}

		// Get the blob content to extract the OID
		blob, _, err := restClient.Git.GetBlob(ctx, owner, name, entry.GetSHA())
		if err != nil {
			// Skip files we can't read
			continue
		}

		// Decode the blob content if it's base64 encoded
		content := blob.GetContent()
		if blob.GetEncoding() == "base64" {
			decoded, err := base64.StdEncoding.DecodeString(content)
			if err != nil {
				// Skip files we can't decode
				continue
			}
			content = string(decoded)
		}

		// If content is empty, the LFS pointer was not migrated properly
		// LFS pointer files should always have content (version, oid, size)
		if strings.TrimSpace(content) == "" {
			// This could be a missing LFS object, but we can't extract the OID
			// Skip for now as we can't add it to the validation list
			continue
		}

		// Check if this is an LFS pointer file and extract the OID
		if lfsObj, isLFS := parseLFSPointer(content); isLFS {
			// Deduplicate by OID
			if !seenOIDs[lfsObj.OID] {
				lfsObjects = append(lfsObjects, lfsObj)
				seenOIDs[lfsObj.OID] = true
			}
		}
	}

	return lfsObjects, nil
}

// getLFSPatternsFromGitAttributes reads .gitattributes and extracts LFS-tracked file patterns
func (api *GitHubAPI) getLFSPatternsFromGitAttributes(ctx context.Context, restClient *github.Client, owner, name, ref string) ([]string, error) {
	// Get the tree to find .gitattributes
	tree, _, err := restClient.Git.GetTree(ctx, owner, name, ref, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get repository tree: %v", err)
	}

	// Look for .gitattributes in the tree
	var gitAttributesSHA string
	for _, entry := range tree.Entries {
		if entry.GetPath() == ".gitattributes" && entry.GetType() == "blob" {
			gitAttributesSHA = entry.GetSHA()
			break
		}
	}

	if gitAttributesSHA == "" {
		return nil, fmt.Errorf(".gitattributes not found")
	}

	// Get the blob content
	blob, _, err := restClient.Git.GetBlob(ctx, owner, name, gitAttributesSHA)
	if err != nil {
		return nil, fmt.Errorf("failed to get .gitattributes blob: %v", err)
	}

	// Decode the content
	content := blob.GetContent()
	if blob.GetEncoding() == "base64" {
		decoded, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			return nil, fmt.Errorf("failed to decode .gitattributes: %v", err)
		}
		content = string(decoded)
	}

	// Parse the .gitattributes file for LFS patterns
	return parseLFSPatterns(content), nil
}

// parseLFSPatterns extracts file patterns that are tracked by LFS from .gitattributes content
func parseLFSPatterns(content string) []string {
	patterns := make([]string, 0)
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Look for lines containing "filter=lfs"
		if strings.Contains(line, "filter=lfs") {
			// Extract the pattern (everything before the first whitespace)
			parts := strings.Fields(line)
			if len(parts) > 0 {
				pattern := parts[0]
				patterns = append(patterns, pattern)
			}
		}
	}

	return patterns
}

// matchesLFSPattern checks if a file path matches any of the LFS patterns
func matchesLFSPattern(filePath string, patterns []string) bool {
	for _, pattern := range patterns {
		// Handle different pattern types
		if strings.HasPrefix(pattern, "*") {
			// Extension pattern like "*.psd"
			if strings.HasSuffix(filePath, strings.TrimPrefix(pattern, "*")) {
				return true
			}
		} else if strings.Contains(pattern, "*") {
			// Glob pattern - simple implementation
			matched, _ := filepath.Match(pattern, filePath)
			if matched {
				return true
			}
			// Also try matching against the base name
			matched, _ = filepath.Match(pattern, filepath.Base(filePath))
			if matched {
				return true
			}
		} else {
			// Exact match or directory pattern
			if filePath == pattern || strings.HasPrefix(filePath, pattern+"/") {
				return true
			}
		}
	}
	return false
}

// getLFSObjectsFallback is the original implementation that checks all small files
// Used as a fallback when .gitattributes cannot be read
func (api *GitHubAPI) getLFSObjectsFallback(ctx context.Context, restClient *github.Client, owner, name, ref, clientName string) ([]LFSObject, error) {
	lfsObjects := make([]LFSObject, 0)
	seenOIDs := make(map[string]bool)

	// Get the repository tree recursively
	tree, _, err := restClient.Git.GetTree(ctx, owner, name, ref, true)
	if err != nil {
		return nil, fmt.Errorf("failed to get %s repository tree: %v", clientName, err)
	}

	// LFS pointer files are always small (less than 200 bytes)
	const maxLFSPointerSize = 200

	// Process each file in the tree
	for _, entry := range tree.Entries {
		// Only process blob entries (files)
		if entry.GetType() != "blob" {
			continue
		}

		// Skip files that are too large to be LFS pointers
		if entry.Size != nil && *entry.Size > maxLFSPointerSize {
			continue
		}

		// Get the blob content
		blob, _, err := restClient.Git.GetBlob(ctx, owner, name, entry.GetSHA())
		if err != nil {
			// Skip files we can't read
			continue
		}

		// Decode the blob content if it's base64 encoded
		content := blob.GetContent()
		if blob.GetEncoding() == "base64" {
			decoded, err := base64.StdEncoding.DecodeString(content)
			if err != nil {
				// Skip files we can't decode
				continue
			}
			content = string(decoded)
		}

		// If content is empty, skip
		if strings.TrimSpace(content) == "" {
			continue
		}

		// Check if this is an LFS pointer file
		if lfsObj, isLFS := parseLFSPointer(content); isLFS {
			// Deduplicate by OID
			if !seenOIDs[lfsObj.OID] {
				lfsObjects = append(lfsObjects, lfsObj)
				seenOIDs[lfsObj.OID] = true
			}
		}
	}

	return lfsObjects, nil
}

// parseLFSPointer parses a blob content to check if it's an LFS pointer file
// and extracts the OID and size if it is
func parseLFSPointer(content string) (LFSObject, bool) {
	// LFS pointer files are small text files with a specific format
	// Example:
	// version https://git-lfs.github.com/spec/v1
	// oid sha256:4d7a214614ab2935c943f9e0ff69d22eadbb8f32b1258daaa5e2ca24d17e2393
	// size 12345

	lines := strings.Split(content, "\n")
	if len(lines) < 3 {
		return LFSObject{}, false
	}

	// Check for LFS version line
	if !strings.HasPrefix(lines[0], "version https://git-lfs.github.com/spec/") {
		return LFSObject{}, false
	}

	var oid string
	var size int64
	sizeFound := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "oid sha256:") {
			oid = strings.TrimPrefix(line, "oid sha256:")
		} else if strings.HasPrefix(line, "size ") {
			// Parse the size and check for errors
			var parsedSize int64
			n, err := fmt.Sscanf(line, "size %d", &parsedSize)
			if err == nil && n == 1 {
				size = parsedSize
				sizeFound = true
			}
		}
	}

	// OID must be present, and size must be found (even if it's 0)
	if oid == "" || !sizeFound {
		return LFSObject{}, false
	}

	return LFSObject{OID: oid, Size: size}, true
}

// ValidateLFSObjects checks if the given LFS objects exist in the repository
// using the Git LFS Batch API
func (api *GitHubAPI) ValidateLFSObjects(clientType ClientType, owner, name string, objects []LFSObject) (int, int, error) {
	if len(objects) == 0 {
		return 0, 0, nil
	}

	config := getClientConfigForType(clientType)

	// Construct the LFS batch API URL
	var lfsURL string
	if config.Hostname != "" {
		hostname := strings.TrimSuffix(config.Hostname, "/")
		if !strings.HasPrefix(hostname, "https://") {
			hostname = "https://" + hostname
		}
		lfsURL = fmt.Sprintf("%s/%s/%s.git/info/lfs/objects/batch", hostname, owner, name)
	} else {
		lfsURL = fmt.Sprintf("https://github.com/%s/%s.git/info/lfs/objects/batch", owner, name)
	}

	// Create the batch request
	batchReq := LFSBatchRequest{
		Operation: "download",
		Transfers: []string{"basic"},
		Objects:   objects,
	}

	reqBody, err := json.Marshal(batchReq)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to marshal LFS batch request: %v", err)
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", lfsURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return 0, 0, fmt.Errorf("failed to create LFS batch request: %v", err)
	}

	req.Header.Set("Accept", "application/vnd.git-lfs+json")
	req.Header.Set("Content-Type", "application/vnd.git-lfs+json")

	// Execute the request using authenticated client
	httpClient, err := createAuthenticatedClient(config)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to create authenticated client: %v", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to execute LFS batch request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return 0, 0, fmt.Errorf("LFS batch API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse the response
	var batchResp LFSBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&batchResp); err != nil {
		return 0, 0, fmt.Errorf("failed to decode LFS batch response: %v", err)
	}

	// Count successful and missing objects
	// An object is considered "existing" only if:
	// 1. It has no error, AND
	// 2. It has download actions (which means the object is actually stored in LFS)
	existingCount := 0
	missingCount := 0

	for _, obj := range batchResp.Objects {
		if obj.Error != nil {
			// Object has an explicit error - definitely missing
			missingCount++
		} else if len(obj.Actions) > 0 && obj.Actions["download"].Href != "" {
			// Object has download actions - it exists in LFS storage
			existingCount++
		} else {
			// No error but also no download actions - object might not exist in LFS storage
			// This can happen when the pointer exists but the actual object wasn't uploaded
			missingCount++
		}
	}

	return existingCount, missingCount, nil
}

// GetLFSObjectCount is a convenience method that gets LFS objects and returns the count
func (api *GitHubAPI) GetLFSObjectCount(clientType ClientType, owner, name string) (int, error) {
	objects, err := api.GetLFSObjects(clientType, owner, name)
	if err != nil {
		return 0, err
	}
	return len(objects), nil
}
