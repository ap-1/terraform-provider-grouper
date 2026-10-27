package grouper

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// GroupPrivilegeNames are the privileges a subject can have on a group.
var GroupPrivilegeNames = []string{"admin", "read", "update", "view", "optin", "optout", "groupAttrRead", "groupAttrUpdate"}

// StemPrivilegeNames are the privileges a subject can have on a folder.
var StemPrivilegeNames = []string{"stemAdmin", "create", "stemView", "stemAttrRead", "stemAttrUpdate"}

// Privilege each of Grouper's privilege lists grants, since Grouper stores grants as memberships
// in these lists
var privilegeLists = map[string]string{
	"admins":            "admin",
	"readers":           "read",
	"updaters":          "update",
	"viewers":           "view",
	"optins":            "optin",
	"optouts":           "optout",
	"groupAttrReaders":  "groupAttrRead",
	"groupAttrUpdaters": "groupAttrUpdate",
	"stemAdmins":        "stemAdmin",
	"creators":          "create",
	"stemViewers":       "stemView",
	"stemAttrReaders":   "stemAttrRead",
	"stemAttrUpdaters":  "stemAttrUpdate",
}

// Group or folder whose privileges are read and set
type privilegeTarget struct {
	uuid string
	// access for groups, naming for folders
	privilegeType string
	names         []string
	// Request field holding the UUID in the lite privilege calls
	uuidField string
	// Request field listing the target in getMemberships
	membershipsField string
}

func groupTarget(uuid string) privilegeTarget {
	return privilegeTarget{uuid, "access", GroupPrivilegeNames, "groupUuid", "wsGroupLookups"}
}

func stemTarget(uuid string) privilegeTarget {
	return privilegeTarget{uuid, "naming", StemPrivilegeNames, "stemUuid", "wsOwnerStemLookups"}
}

// GroupPrivileges returns the privileges subject has directly on the group with the given UUID.
func (c *Client) GroupPrivileges(ctx context.Context, uuid string, subject Subject) ([]string, error) {
	return c.privileges(ctx, groupTarget(uuid), subject)
}

// StemPrivileges returns the privileges subject has directly on the folder with the given UUID.
func (c *Client) StemPrivileges(ctx context.Context, uuid string, subject Subject) ([]string, error) {
	return c.privileges(ctx, stemTarget(uuid), subject)
}

// SetGroupPrivileges makes privileges the exact privileges subject has directly on the group with
// the given UUID, leaving other subjects' privileges alone.
func (c *Client) SetGroupPrivileges(ctx context.Context, uuid string, subject Subject, privileges []string) error {
	return c.setPrivileges(ctx, groupTarget(uuid), subject, privileges)
}

// SetStemPrivileges makes privileges the exact privileges subject has directly on the folder with
// the given UUID, leaving other subjects' privileges alone.
func (c *Client) SetStemPrivileges(ctx context.Context, uuid string, subject Subject, privileges []string) error {
	return c.setPrivileges(ctx, stemTarget(uuid), subject, privileges)
}

// Privileges granted to subject itself, not those it inherits through GrouperAll or a group. The
// lite privileges call can't tell these apart, since it returns one effective grant per privilege.
// A subject that doesn't exist has none.
func (c *Client) privileges(ctx context.Context, target privilegeTarget, subject Subject) ([]string, error) {
	req := map[string]any{
		"WsRestGetMembershipsRequest": map[string]any{
			target.membershipsField: []map[string]string{{"uuid": target.uuid}},
			"fieldType":             target.privilegeType,
			"memberFilter":          "Immediate",
			"wsSubjectLookups":      []Subject{subject},
		},
	}

	var resp struct {
		WsGetMembershipsResults struct {
			WsMemberships []struct {
				ListName string `json:"listName"`
			} `json:"wsMemberships"`
		} `json:"WsGetMembershipsResults"`
	}
	if err := c.post(ctx, "memberships", req, &resp); err != nil {
		// Grouper reports a missing folder here as a generic EXCEPTION
		var e *Error
		if errors.As(err, &e) && strings.HasPrefix(e.Message, "Could not find stem") {
			e.Code = "STEM_NOT_FOUND"
		}
		return nil, err
	}

	var names []string
	for _, m := range resp.WsGetMembershipsResults.WsMemberships {
		if name, ok := privilegeLists[m.ListName]; ok {
			names = append(names, name)
		}
	}
	return names, nil
}

func (c *Client) setPrivileges(ctx context.Context, target privilegeTarget, subject Subject, want []string) error {
	for _, name := range want {
		if !slices.Contains(target.names, name) {
			return fmt.Errorf("grouper: unknown %s privilege %q, expected one of %v", target.privilegeType, name, target.names)
		}
	}

	current, err := c.privileges(ctx, target, subject)
	if err != nil {
		return err
	}

	for _, name := range current {
		if !slices.Contains(want, name) {
			if err := c.assignPrivilege(ctx, target, subject, name, false); err != nil {
				return err
			}
		}
	}
	for _, name := range want {
		if !slices.Contains(current, name) {
			if err := c.assignPrivilege(ctx, target, subject, name, true); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Client) assignPrivilege(ctx context.Context, target privilegeTarget, subject Subject, name string, allowed bool) error {
	req := map[string]string{
		"subjectId":     subject.ID,
		"privilegeType": target.privilegeType,
		"privilegeName": name,
		"allowed":       "F",
	}
	if allowed {
		req["allowed"] = "T"
	}
	if subject.SourceID != "" {
		req["subjectSourceId"] = subject.SourceID
	}
	req[target.uuidField] = target.uuid

	return c.post(ctx, "grouperPrivileges", map[string]any{"WsRestAssignGrouperPrivilegesLiteRequest": req}, &struct{}{})
}
