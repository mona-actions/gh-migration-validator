package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

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
	OID   string                `json:"oid"`
	Size  int64                 `json:"size"`
	Error *LFSBatchObjectError  `json:"error,omitempty"`
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
// by traversing the repository tree and parsing LFS pointer files
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

	// Use REST API to get all LFS pointer files
	restClient, _, err := api.getRESTClient(clientType)
	if err != nil {
		return nil, err
	}

	lfsObjects := make([]LFSObject, 0)
	seenOIDs := make(map[string]bool) // Track OIDs to deduplicate
	
	// Get the repository tree recursively
	tree, _, err := restClient.Git.GetTree(ctx, owner, name, defaultBranch, true)
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
	existingCount := 0
	missingCount := 0

	for _, obj := range batchResp.Objects {
		if obj.Error != nil {
			// Object has an error
			if obj.Error.Code == 404 {
				missingCount++
			} else {
				// Other errors (403, 500, etc.) are also considered missing/unavailable
				missingCount++
			}
		} else {
			// Object exists (may or may not have download actions)
			existingCount++
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
