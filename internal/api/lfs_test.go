package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseLFSPointer_ValidPointer(t *testing.T) {
	content := `version https://git-lfs.github.com/spec/v1
oid sha256:4d7a214614ab2935c943f9e0ff69d22eadbb8f32b1258daaa5e2ca24d17e2393
size 12345`

	obj, isLFS := parseLFSPointer(content)

	assert.True(t, isLFS, "Should identify valid LFS pointer")
	assert.Equal(t, "4d7a214614ab2935c943f9e0ff69d22eadbb8f32b1258daaa5e2ca24d17e2393", obj.OID)
	assert.Equal(t, int64(12345), obj.Size)
}

func TestParseLFSPointer_ValidPointerWithExtraWhitespace(t *testing.T) {
	content := `version https://git-lfs.github.com/spec/v1
oid sha256:4d7a214614ab2935c943f9e0ff69d22eadbb8f32b1258daaa5e2ca24d17e2393  
size 12345  
`

	obj, isLFS := parseLFSPointer(content)

	assert.True(t, isLFS, "Should identify valid LFS pointer with extra whitespace")
	assert.Equal(t, "4d7a214614ab2935c943f9e0ff69d22eadbb8f32b1258daaa5e2ca24d17e2393", obj.OID)
	assert.Equal(t, int64(12345), obj.Size)
}

func TestParseLFSPointer_ValidPointerWithZeroSize(t *testing.T) {
	content := `version https://git-lfs.github.com/spec/v1
oid sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
size 0`

	obj, isLFS := parseLFSPointer(content)

	assert.True(t, isLFS, "Should identify valid LFS pointer with size 0 (empty file)")
	assert.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", obj.OID)
	assert.Equal(t, int64(0), obj.Size)
}

func TestParseLFSPointer_NotLFSPointer(t *testing.T) {
	content := `This is just a regular text file
with multiple lines
and no LFS pointer format`

	_, isLFS := parseLFSPointer(content)

	assert.False(t, isLFS, "Should not identify regular file as LFS pointer")
}

func TestParseLFSPointer_InvalidVersionLine(t *testing.T) {
	content := `version https://example.com/spec/v1
oid sha256:4d7a214614ab2935c943f9e0ff69d22eadbb8f32b1258daaa5e2ca24d17e2393
size 12345`

	_, isLFS := parseLFSPointer(content)

	assert.False(t, isLFS, "Should not identify file with invalid version as LFS pointer")
}

func TestParseLFSPointer_MissingOID(t *testing.T) {
	content := `version https://git-lfs.github.com/spec/v1
size 12345`

	_, isLFS := parseLFSPointer(content)

	assert.False(t, isLFS, "Should not identify pointer without OID as valid")
}

func TestParseLFSPointer_MissingSize(t *testing.T) {
	content := `version https://git-lfs.github.com/spec/v1
oid sha256:4d7a214614ab2935c943f9e0ff69d22eadbb8f32b1258daaa5e2ca24d17e2393`

	_, isLFS := parseLFSPointer(content)

	assert.False(t, isLFS, "Should not identify pointer without size as valid")
}

func TestParseLFSPointer_MalformedSize(t *testing.T) {
	content := `version https://git-lfs.github.com/spec/v1
oid sha256:4d7a214614ab2935c943f9e0ff69d22eadbb8f32b1258daaa5e2ca24d17e2393
size abc`

	_, isLFS := parseLFSPointer(content)

	assert.False(t, isLFS, "Should not identify pointer with malformed size as valid")
}

func TestParseLFSPointer_TooShort(t *testing.T) {
	content := `version https://git-lfs.github.com/spec/v1
size 12345`

	_, isLFS := parseLFSPointer(content)

	assert.False(t, isLFS, "Should not identify file with too few lines as LFS pointer")
}

func TestParseLFSPointer_EmptyContent(t *testing.T) {
	content := ``

	_, isLFS := parseLFSPointer(content)

	assert.False(t, isLFS, "Should not identify empty content as LFS pointer")
}

func TestLFSObject_Structure(t *testing.T) {
	// Test that LFSObject can be properly created and accessed
	obj := LFSObject{
		OID:  "abc123",
		Size: 12345,
	}

	assert.Equal(t, "abc123", obj.OID)
	assert.Equal(t, int64(12345), obj.Size)
}

func TestLFSBatchRequest_Structure(t *testing.T) {
	// Test that LFSBatchRequest can be properly created
	req := LFSBatchRequest{
		Operation: "download",
		Transfers: []string{"basic"},
		Objects: []LFSObject{
			{OID: "oid1", Size: 100},
			{OID: "oid2", Size: 200},
		},
	}

	assert.Equal(t, "download", req.Operation)
	assert.Equal(t, []string{"basic"}, req.Transfers)
	assert.Len(t, req.Objects, 2)
	assert.Equal(t, "oid1", req.Objects[0].OID)
	assert.Equal(t, int64(100), req.Objects[0].Size)
}

func TestLFSBatchResponse_Structure(t *testing.T) {
	// Test that LFSBatchResponse can be properly created
	resp := LFSBatchResponse{
		Objects: []LFSBatchObject{
			{
				OID:  "oid1",
				Size: 100,
				Actions: map[string]LFSAction{
					"download": {Href: "https://example.com/download"},
				},
			},
			{
				OID:  "oid2",
				Size: 200,
				Error: &LFSBatchObjectError{
					Code:    404,
					Message: "Object not found",
				},
			},
		},
	}

	assert.Len(t, resp.Objects, 2)
	assert.NotNil(t, resp.Objects[0].Actions)
	assert.Nil(t, resp.Objects[0].Error)
	assert.NotNil(t, resp.Objects[1].Error)
	assert.Equal(t, 404, resp.Objects[1].Error.Code)
}
