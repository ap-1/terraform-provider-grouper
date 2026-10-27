package grouper

import (
	"context"
	"net/http"
	"strconv"
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
	// Set when the group's members come from combining two other groups, which also means it can't
	// have members of its own
	Composite *Composite `json:"-"`
}

// Composite combines two groups' members into a third group's.
type Composite struct {
	// union, intersection, or complement, which is Left's members who aren't in Right
	Type      string
	LeftUUID  string
	RightUUID string
}

// Group in the shape Grouper's web services send and receive
type wsGroup struct {
	Group
	Detail *wsGroupDetail `json:"detail,omitempty"`
}

type wsGroupDetail struct {
	// T or F
	HasComposite  string         `json:"hasComposite"`
	CompositeType string         `json:"compositeType,omitempty"`
	LeftGroup     *wsGroupLookup `json:"leftGroup,omitempty"`
	RightGroup    *wsGroupLookup `json:"rightGroup,omitempty"`
}

type wsGroupLookup struct {
	UUID string `json:"uuid"`
}

func toWSGroup(group Group) wsGroup {
	ws := wsGroup{Group: group, Detail: &wsGroupDetail{HasComposite: "F"}}
	if c := group.Composite; c != nil {
		ws.Detail = &wsGroupDetail{
			HasComposite:  "T",
			CompositeType: c.Type,
			LeftGroup:     &wsGroupLookup{UUID: c.LeftUUID},
			RightGroup:    &wsGroupLookup{UUID: c.RightUUID},
		}
	}
	return ws
}

func (ws wsGroup) group() *Group {
	group := ws.Group
	if d := ws.Detail; d != nil && d.HasComposite == "T" && d.LeftGroup != nil && d.RightGroup != nil {
		group.Composite = &Composite{Type: d.CompositeType, LeftUUID: d.LeftGroup.UUID, RightUUID: d.RightGroup.UUID}
	}
	return &group
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

// Groups fetched per request by ListGroups
const listPageSize = 100

// ListGroups returns the ordinary groups directly in the folder named folder, sorted by name, or
// also those in its subfolders at any depth when recursive is set. A folder that doesn't exist
// has none.
func (c *Client) ListGroups(ctx context.Context, folder string, recursive bool) ([]Group, error) {
	scope := "ONE_LEVEL"
	if recursive {
		scope = "ALL_IN_SUBTREE"
	}

	var groups []Group
	// Stops at an empty page, since a short one might only mean Grouper capped the page size
	for page := 1; ; page++ {
		results, err := c.findGroups(ctx, map[string]any{
			"wsQueryFilter": map[string]string{
				"queryFilterType": "FIND_BY_STEM_NAME",
				"stemName":        folder,
				"stemNameScope":   scope,
				"typeOfGroups":    "group",
				"sortString":      "name",
				"ascending":       "T",
				"pageSize":        strconv.Itoa(listPageSize),
				"pageNumber":      strconv.Itoa(page),
			},
		})
		if err != nil {
			return nil, err
		}
		if len(results) == 0 {
			return groups, nil
		}

		for _, ws := range results {
			groups = append(groups, *ws.group())
		}
	}
}

func (c *Client) findGroup(ctx context.Context, lookup map[string]string) (*Group, error) {
	groups, err := c.findGroups(ctx, map[string]any{"wsGroupLookups": []map[string]string{lookup}})
	if err != nil || len(groups) == 0 {
		return nil, err
	}
	return groups[0].group(), nil
}

// Groups matching a find request's lookups or query filter, with their composite settings
func (c *Client) findGroups(ctx context.Context, request map[string]any) ([]wsGroup, error) {
	request["includeGroupDetail"] = "T"

	var resp struct {
		WsFindGroupsResults struct {
			GroupResults []wsGroup `json:"groupResults"`
		} `json:"WsFindGroupsResults"`
	}
	if err := c.post(ctx, "groups", map[string]any{"WsRestFindGroupsRequest": request}, &resp); err != nil {
		return nil, err
	}
	return resp.WsFindGroupsResults.GroupResults, nil
}

// CreateGroup creates a group, failing if one with the same name exists or its folder doesn't.
// A composite group's operand groups must already exist.
func (c *Client) CreateGroup(ctx context.Context, group Group) (*Group, error) {
	return c.saveGroup(ctx, map[string]string{"groupName": group.Name}, group, "INSERT")
}

// UpdateGroup changes the group with the given UUID, renaming it if group.Name differs, and
// making it composite or not to match group.Composite. Grouper can't move a group to another
// folder, or make a group with members composite.
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
				"wsGroup":       toWSGroup(group),
				"saveMode":      saveMode,
			}},
			"includeGroupDetail": "T",
		},
	}

	var resp struct {
		WsGroupSaveResults struct {
			Results []struct {
				WsGroup wsGroup `json:"wsGroup"`
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
	return results[0].WsGroup.group(), nil
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
