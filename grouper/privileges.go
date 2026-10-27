package grouper

import (
	"context"
	"fmt"
	"slices"
)

// GroupPrivilegeNames are the privileges a subject can have on a group.
var GroupPrivilegeNames = []string{"admin", "read", "update", "view", "optin", "optout", "groupAttrRead", "groupAttrUpdate"}

// StemPrivilegeNames are the privileges a subject can have on a folder.
var StemPrivilegeNames = []string{"stemAdmin", "create", "stemView", "stemAttrRead", "stemAttrUpdate"}

// Group or folder whose privileges are read and set
type privilegeTarget struct {
	lookup map[string]string
	// access for groups, naming for folders
	privilegeType string
	names         []string
}

func groupTarget(uuid string) privilegeTarget {
	return privilegeTarget{map[string]string{"groupUuid": uuid}, "access", GroupPrivilegeNames}
}

func stemTarget(uuid string) privilegeTarget {
	return privilegeTarget{map[string]string{"stemUuid": uuid}, "naming", StemPrivilegeNames}
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

func (c *Client) privileges(ctx context.Context, target privilegeTarget, subject Subject) ([]string, error) {
	req := map[string]string{
		"subjectId":     subject.ID,
		"privilegeType": target.privilegeType,
	}
	if subject.SourceID != "" {
		req["subjectSourceId"] = subject.SourceID
	}
	for k, v := range target.lookup {
		req[k] = v
	}

	var resp struct {
		WsGetGrouperPrivilegesLiteResult struct {
			PrivilegeResults []struct {
				PrivilegeName string `json:"privilegeName"`
				Allowed       string `json:"allowed"`
			} `json:"privilegeResults"`
		} `json:"WsGetGrouperPrivilegesLiteResult"`
	}
	if err := c.post(ctx, "grouperPrivileges", map[string]any{"WsRestGetGrouperPrivilegesLiteRequest": req}, &resp); err != nil {
		return nil, err
	}

	var names []string
	for _, p := range resp.WsGetGrouperPrivilegesLiteResult.PrivilegeResults {
		if p.Allowed == "T" {
			names = append(names, p.PrivilegeName)
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
	for k, v := range target.lookup {
		req[k] = v
	}

	return c.post(ctx, "grouperPrivileges", map[string]any{"WsRestAssignGrouperPrivilegesLiteRequest": req}, &struct{}{})
}
