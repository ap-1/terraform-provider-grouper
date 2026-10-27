package grouper

import (
	"context"
	"net/http"
)

// Stem is a Grouper folder.
type Stem struct {
	// Full name, such as org:dept:apps
	Name string `json:"name"`
	// Last segment of DisplayName, which Grouper sets to the last segment of Name when empty
	DisplayExtension string `json:"displayExtension,omitempty"`
	// Full name with each segment's display name
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description"`
	UUID        string `json:"uuid,omitempty"`
}

// FindStem returns the folder whose current full name is name, or nil when there is none.
// Grouper also matches a folder's former names after a rename, but FindStem doesn't.
func (c *Client) FindStem(ctx context.Context, name string) (*Stem, error) {
	stem, err := c.findStem(ctx, map[string]string{
		"stemName":            name,
		"stemQueryFilterType": "FIND_BY_STEM_NAME",
	})
	if err != nil || stem == nil || stem.Name != name {
		return nil, err
	}
	return stem, nil
}

// FindStemByUUID returns the folder with the given UUID, or nil when there is none.
func (c *Client) FindStemByUUID(ctx context.Context, uuid string) (*Stem, error) {
	return c.findStem(ctx, map[string]string{
		"stemUuid":            uuid,
		"stemQueryFilterType": "FIND_BY_STEM_UUID",
	})
}

func (c *Client) findStem(ctx context.Context, filter map[string]string) (*Stem, error) {
	req := map[string]any{
		"WsRestFindStemsRequest": map[string]any{
			"wsStemQueryFilter": filter,
		},
	}

	var resp struct {
		WsFindStemsResults struct {
			StemResults []Stem `json:"stemResults"`
		} `json:"WsFindStemsResults"`
	}
	if err := c.post(ctx, "stems", req, &resp); err != nil {
		return nil, err
	}

	stems := resp.WsFindStemsResults.StemResults
	if len(stems) == 0 {
		return nil, nil
	}
	return &stems[0], nil
}

// CreateStem creates a folder, failing if one with the same name exists or its parent doesn't.
func (c *Client) CreateStem(ctx context.Context, stem Stem) (*Stem, error) {
	return c.saveStem(ctx, map[string]string{"stemName": stem.Name}, stem, "INSERT")
}

// UpdateStem changes the folder with the given UUID, renaming it if stem.Name differs.
// Grouper can't move a folder to another parent.
func (c *Client) UpdateStem(ctx context.Context, uuid string, stem Stem) (*Stem, error) {
	return c.saveStem(ctx, map[string]string{"uuid": uuid}, stem, "UPDATE")
}

func (c *Client) saveStem(ctx context.Context, lookup map[string]string, stem Stem, saveMode string) (*Stem, error) {
	stem.DisplayName = ""
	stem.UUID = ""
	req := map[string]any{
		"WsRestStemSaveRequest": map[string]any{
			"wsStemToSaves": []map[string]any{{
				"wsStemLookup": lookup,
				"wsStem":       stem,
				"saveMode":     saveMode,
			}},
		},
	}

	var resp struct {
		WsStemSaveResults struct {
			Results []struct {
				WsStem Stem `json:"wsStem"`
			} `json:"results"`
		} `json:"WsStemSaveResults"`
	}
	if err := c.post(ctx, "stems", req, &resp); err != nil {
		return nil, err
	}

	results := resp.WsStemSaveResults.Results
	if len(results) == 0 {
		return nil, &Error{Status: http.StatusOK, Message: "stem save returned no results"}
	}
	return &results[0].WsStem, nil
}

// DeleteStem deletes the folder with the given UUID, succeeding if it doesn't exist.
// Grouper refuses to delete a folder that isn't empty.
func (c *Client) DeleteStem(ctx context.Context, uuid string) error {
	req := map[string]any{
		"WsRestStemDeleteRequest": map[string]any{
			"wsStemLookups": []map[string]string{{"uuid": uuid}},
		},
	}
	return c.post(ctx, "stems", req, &struct{}{})
}
