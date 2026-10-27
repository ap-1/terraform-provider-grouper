package grouper

import (
	"context"
	"net/http"
	"strings"
)

// GroupSource is the subject source of groups, whose subject IDs are their UUIDs.
const GroupSource = "g:gsa"

// Subject is a member of a group, such as a person or another group.
type Subject struct {
	ID string `json:"subjectId"`
	// Source the subject comes from, such as ldap or GroupSource, or empty to search every source
	SourceID string `json:"subjectSourceId,omitempty"`
}

// MemberFilter chooses which of a group's members GroupMembers returns.
type MemberFilter string

const (
	// ImmediateMembers are the subjects and groups added to the group directly.
	ImmediateMembers MemberFilter = "Immediate"
	// AllMembers are the immediate members plus everyone in nested groups, at any depth.
	AllMembers MemberFilter = "All"
)

// GroupMembers returns the members of the group with the given UUID that filter selects.
func (c *Client) GroupMembers(ctx context.Context, uuid string, filter MemberFilter) ([]Subject, error) {
	req := map[string]any{
		"WsRestGetMembersRequest": map[string]any{
			"wsGroupLookups": []map[string]string{{"uuid": uuid}},
			"memberFilter":   filter,
		},
	}

	var resp struct {
		WsGetMembersResults struct {
			Results []struct {
				WsSubjects []struct {
					ID       string `json:"id"`
					SourceID string `json:"sourceId"`
				} `json:"wsSubjects"`
			} `json:"results"`
		} `json:"WsGetMembersResults"`
	}
	if err := c.post(ctx, "groups", req, &resp); err != nil {
		return nil, err
	}

	results := resp.WsGetMembersResults.Results
	if len(results) == 0 {
		return nil, &Error{Status: http.StatusOK, Message: "get members returned no results"}
	}

	members := make([]Subject, len(results[0].WsSubjects))
	for i, s := range results[0].WsSubjects {
		members[i] = Subject{ID: s.ID, SourceID: s.SourceID}
	}
	return members, nil
}

// SetGroupMembers makes members the immediate members of the group with the given UUID, removing
// everyone else. It changes nothing if any member can't be found, since Grouper would otherwise
// apply the rest of the list.
func (c *Client) SetGroupMembers(ctx context.Context, uuid string, members []Subject) error {
	if err := c.checkSubjectsExist(ctx, members); err != nil {
		return err
	}

	// Grouper clears the group for an empty list, so it must not be sent as null
	if members == nil {
		members = []Subject{}
	}

	req := map[string]any{
		"WsRestAddMemberRequest": map[string]any{
			"wsGroupLookup":      map[string]string{"uuid": uuid},
			"subjectLookups":     members,
			"replaceAllExisting": "T",
		},
	}
	return c.post(ctx, "groups", req, &struct{}{})
}

func (c *Client) checkSubjectsExist(ctx context.Context, subjects []Subject) error {
	if len(subjects) == 0 {
		return nil
	}

	req := map[string]any{
		"WsRestGetSubjectsRequest": map[string]any{
			"wsSubjectLookups": subjects,
		},
	}

	var resp struct {
		WsGetSubjectsResults struct {
			WsSubjects []struct {
				ID      string `json:"id"`
				Success string `json:"success"`
			} `json:"wsSubjects"`
		} `json:"WsGetSubjectsResults"`
	}
	if err := c.post(ctx, "subjects", req, &resp); err != nil {
		return err
	}

	var missing []string
	for _, s := range resp.WsGetSubjectsResults.WsSubjects {
		if s.Success != "T" {
			missing = append(missing, s.ID)
		}
	}
	if len(missing) > 0 {
		return &Error{Status: http.StatusOK, Code: "SUBJECT_NOT_FOUND", Message: "can't find subjects " + strings.Join(missing, ", ")}
	}
	return nil
}
