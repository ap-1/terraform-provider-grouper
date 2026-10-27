package grouper

import (
	"context"
	"net/http"
)

// Group is a Grouper group.
type Group struct {
	// Full name, such as org:dept:apps:admins
	Name string `json:"name"`
	// Last segment of DisplayName, which Grouper sets to the last segment of Name when empty
	DisplayExtension string `json:"displayExtension,omitempty"`
	// Full name with each segment's display name
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description"`
	UUID        string `json:"uuid,omitempty"`
}

// FindGroup returns the group whose current full name is name, or nil when there is none.
// Grouper also matches a group's former names after a rename, but FindGroup doesn't.
func (c *Client) FindGroup(ctx context.Context, name string) (*Group, error) {
	group, err := c.findGroup(ctx, map[string]string{"groupName": name})
	if err != nil || group == nil || group.Name != name {
		return nil, err
	}
	return group, nil
}

// FindGroupByUUID returns the group with the given UUID, or nil when there is none.
func (c *Client) FindGroupByUUID(ctx context.Context, uuid string) (*Group, error) {
	return c.findGroup(ctx, map[string]string{"uuid": uuid})
}

func (c *Client) findGroup(ctx context.Context, lookup map[string]string) (*Group, error) {
	req := map[string]any{
		"WsRestFindGroupsRequest": map[string]any{
			"wsGroupLookups": []map[string]string{lookup},
		},
	}

	var resp struct {
		WsFindGroupsResults struct {
			GroupResults []Group `json:"groupResults"`
		} `json:"WsFindGroupsResults"`
	}
	if err := c.post(ctx, "groups", req, &resp); err != nil {
		return nil, err
	}

	groups := resp.WsFindGroupsResults.GroupResults
	if len(groups) == 0 {
		return nil, nil
	}
	return &groups[0], nil
}

// CreateGroup creates a group, failing if one with the same name exists or its folder doesn't.
func (c *Client) CreateGroup(ctx context.Context, group Group) (*Group, error) {
	return c.saveGroup(ctx, map[string]string{"groupName": group.Name}, group, "INSERT")
}

// UpdateGroup changes the group with the given UUID, renaming it if group.Name differs.
// Grouper can't move a group to another folder.
func (c *Client) UpdateGroup(ctx context.Context, uuid string, group Group) (*Group, error) {
	return c.saveGroup(ctx, map[string]string{"uuid": uuid}, group, "UPDATE")
}

func (c *Client) saveGroup(ctx context.Context, lookup map[string]string, group Group, saveMode string) (*Group, error) {
	group.DisplayName = ""
	group.UUID = ""
	req := map[string]any{
		"WsRestGroupSaveRequest": map[string]any{
			"wsGroupToSaves": []map[string]any{{
				"wsGroupLookup": lookup,
				"wsGroup":       group,
				"saveMode":      saveMode,
			}},
		},
	}

	var resp struct {
		WsGroupSaveResults struct {
			Results []struct {
				WsGroup Group `json:"wsGroup"`
			} `json:"results"`
		} `json:"WsGroupSaveResults"`
	}
	if err := c.post(ctx, "groups", req, &resp); err != nil {
		return nil, err
	}

	results := resp.WsGroupSaveResults.Results
	if len(results) == 0 {
		return nil, &Error{Status: http.StatusOK, Message: "group save returned no results"}
	}
	return &results[0].WsGroup, nil
}

// DeleteGroup deletes the group with the given UUID, succeeding if it doesn't exist.
func (c *Client) DeleteGroup(ctx context.Context, uuid string) error {
	req := map[string]any{
		"WsRestGroupDeleteRequest": map[string]any{
			"wsGroupLookups": []map[string]string{{"uuid": uuid}},
		},
	}
	return c.post(ctx, "groups", req, &struct{}{})
}
