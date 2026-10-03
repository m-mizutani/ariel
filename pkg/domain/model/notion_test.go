package model_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/robin/pkg/domain/model"
)

func TestNotionWorkspaceID_Validate(t *testing.T) {
	gt.NoError(t, model.NotionWorkspaceID("0f4a2b1c-3d4e-4f50-8a6b-7c8d9e0f1a2b").Validate())

	for name, id := range map[string]string{
		"empty":      "",
		"no hyphens": "0f4a2b1c3d4e4f508a6b7c8d9e0f1a2b",
		"upper case": "0F4A2B1C-3D4E-4F50-8A6B-7C8D9E0F1A2B",
		"too short":  "0f4a2b1c-3d4e-4f50-8a6b-7c8d9e0f1a2",
	} {
		t.Run(name, func(t *testing.T) {
			gt.Error(t, model.NotionWorkspaceID(id).Validate())
		})
	}
}

func TestNotionObjectID_Validate(t *testing.T) {
	for _, id := range []string{
		"0f4a2b1c-3d4e-4f50-8a6b-7c8d9e0f1a2b",
		"0f4a2b1c3d4e4f508a6b7c8d9e0f1a2b",
		"0F4A2B1C3D4E4F508A6B7C8D9E0F1A2B",
	} {
		t.Run("valid "+id, func(t *testing.T) {
			gt.NoError(t, model.NotionObjectID(id).Validate())
		})
	}

	for name, id := range map[string]string{
		"empty":             "",
		"31 digits":         "0f4a2b1c3d4e4f508a6b7c8d9e0f1a2",
		"not hex":           "0f4a2b1c3d4e4f508a6b7c8d9e0f1a2z",
		"path traversal":    "../x",
		"misplaced hyphens": "0f4a2b1c3-d4e-4f50-8a6b-7c8d9e0f1a2b",
		"trailing slash":    "0f4a2b1c3d4e4f508a6b7c8d9e0f1a2b/",
	} {
		t.Run(name, func(t *testing.T) {
			gt.Error(t, model.NotionObjectID(id).Validate())
		})
	}
}

func TestNotionUserID_Validate(t *testing.T) {
	gt.NoError(t, model.NotionUserID("7b3c1e2d-4f5a-4b6c-9d8e-1f2a3b4c5d6e").Validate())
	gt.Error(t, model.NotionUserID("").Validate())
	gt.Error(t, model.NotionUserID("users/other").Validate())
}

func validNotionCredential() *model.NotionCredential {
	now := time.Now()
	return &model.NotionCredential{
		TeamID: "T0123ABCD",
		UserID: "U0123ABCD",
		Tokens: model.EncryptedData{
			KeyName:    "projects/p/locations/l/keyRings/r/cryptoKeys/k",
			Ciphertext: []byte("ciphertext"),
		},
		WorkspaceID:    "0f4a2b1c-3d4e-4f50-8a6b-7c8d9e0f1a2b",
		WorkspaceName:  "Example",
		BotID:          "b1c2d3e4-0000-4000-8000-000000000001",
		NotionUserID:   "7b3c1e2d-4f5a-4b6c-9d8e-1f2a3b4c5d6e",
		NotionUserName: "Alice Example",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func TestNotionCredential_Validate(t *testing.T) {
	gt.NoError(t, validNotionCredential().Validate())

	t.Run("names may be empty", func(t *testing.T) {
		c := validNotionCredential()
		c.WorkspaceName = ""
		c.NotionUserName = ""
		gt.NoError(t, c.Validate())
	})

	cases := map[string]func(c *model.NotionCredential){
		"empty team":        func(c *model.NotionCredential) { c.TeamID = "" },
		"invalid user":      func(c *model.NotionCredential) { c.UserID = "alice" },
		"empty key name":    func(c *model.NotionCredential) { c.Tokens.KeyName = "" },
		"empty ciphertext":  func(c *model.NotionCredential) { c.Tokens.Ciphertext = nil },
		"empty workspace":   func(c *model.NotionCredential) { c.WorkspaceID = "" },
		"empty bot":         func(c *model.NotionCredential) { c.BotID = "" },
		"empty notion user": func(c *model.NotionCredential) { c.NotionUserID = "" },
		"empty created_at":  func(c *model.NotionCredential) { c.CreatedAt = time.Time{} },
		"empty updated_at":  func(c *model.NotionCredential) { c.UpdatedAt = time.Time{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := validNotionCredential()
			mutate(c)
			gt.Error(t, c.Validate())
		})
	}
}

func TestNotionCredential_Key(t *testing.T) {
	gt.Value(t, validNotionCredential().Key()).Equal(model.UserKey{TeamID: "T0123ABCD", UserID: "U0123ABCD"})
}

func TestNotionPagination_Validate(t *testing.T) {
	gt.NoError(t, model.NotionPagination{PageSize: 0}.Validate())
	gt.NoError(t, model.NotionPagination{PageSize: 100, StartCursor: "c"}.Validate())
	gt.Error(t, model.NotionPagination{PageSize: -1}.Validate())
	gt.Error(t, model.NotionPagination{PageSize: 101}.Validate())
}

func TestNotionSearchQuery_Validate(t *testing.T) {
	for _, typ := range []model.NotionObjectType{"", "page", "data_source"} {
		gt.NoError(t, model.NotionSearchQuery{Query: "roadmap", ObjectType: typ}.Validate())
	}
	gt.Error(t, model.NotionSearchQuery{ObjectType: "database"}.Validate())
	gt.Error(t, model.NotionSearchQuery{Page: model.NotionPagination{PageSize: 101}}.Validate())
}

func TestNotionDataSourceQuery_Validate(t *testing.T) {
	gt.NoError(t, model.NotionDataSourceQuery{}.Validate())
	gt.NoError(t, model.NotionDataSourceQuery{
		Filter: json.RawMessage(`{"property":"Status","status":{"equals":"Done"}}`),
		Sorts:  json.RawMessage(`[{"timestamp":"last_edited_time","direction":"descending"}]`),
	}.Validate())
	gt.Error(t, model.NotionDataSourceQuery{Filter: json.RawMessage(`{"property":`)}.Validate())
	gt.Error(t, model.NotionDataSourceQuery{Sorts: json.RawMessage(`[`)}.Validate())
	gt.Error(t, model.NotionDataSourceQuery{Page: model.NotionPagination{PageSize: -1}}.Validate())
}
