package grouper

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
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

// Membership is a subject's immediate membership in a group, which Grouper can start and end on
// set dates.
type Membership struct {
	Subject
	// Zero when the membership has no start date
	Starts time.Time
	// Zero when the membership has no end date
	Ends time.Time
}

// GroupMemberships returns the immediate memberships of the group with the given UUID, including
// those that haven't started yet or have ended.
func (c *Client) GroupMemberships(ctx context.Context, uuid string) ([]Membership, error) {
	req := map[string]any{
		"WsRestGetMembershipsRequest": map[string]any{
			"wsGroupLookups": []map[string]string{{"uuid": uuid}},
			"fieldName":      "members",
			"memberFilter":   "Immediate",
			// T, F, or A for both enabled and disabled
			"enabled": "A",
		},
	}

	var resp struct {
		WsGetMembershipsResults struct {
			WsMemberships []struct {
				SubjectID       string `json:"subjectId"`
				SubjectSourceID string `json:"subjectSourceId"`
				EnabledTime     string `json:"enabledTime"`
				DisabledTime    string `json:"disabledTime"`
			} `json:"wsMemberships"`
		} `json:"WsGetMembershipsResults"`
	}
	if err := c.post(ctx, "memberships", req, &resp); err != nil {
		return nil, err
	}

	memberships := make([]Membership, len(resp.WsGetMembershipsResults.WsMemberships))
	for i, m := range resp.WsGetMembershipsResults.WsMemberships {
		starts, err := c.parseTime(m.EnabledTime)
		if err != nil {
			return nil, err
		}
		ends, err := c.parseTime(m.DisabledTime)
		if err != nil {
			return nil, err
		}
		memberships[i] = Membership{Subject: Subject{ID: m.SubjectID, SourceID: m.SubjectSourceID}, Starts: starts, Ends: ends}
	}
	return memberships, nil
}

// SetGroupMemberships makes memberships the only immediate members of the group with the given
// UUID. Every subject must have a SourceID, and if any subject doesn't exist, the group is left
// unchanged.
func (c *Client) SetGroupMemberships(ctx context.Context, uuid string, memberships []Membership) error {
	subjects := make([]Subject, len(memberships))
	for i, m := range memberships {
		if m.SourceID == "" {
			return fmt.Errorf("grouper: subject %q has no source", m.ID)
		}
		subjects[i] = m.Subject
	}
	if err := c.checkSubjectsExist(ctx, subjects); err != nil {
		return err
	}

	current, err := c.GroupMemberships(ctx, uuid)
	if err != nil {
		return err
	}
	currentBySubject := make(map[Subject]Membership, len(current))
	for _, m := range current {
		currentBySubject[m.Subject] = m
	}

	wanted := make(map[Subject]bool, len(memberships))
	for _, m := range memberships {
		wanted[m.Subject] = true
	}
	// Removed first, so stopping partway leaves less access
	var remove []Subject
	for _, m := range current {
		if !wanted[m.Subject] {
			remove = append(remove, m.Subject)
		}
	}
	if err := c.deleteMembers(ctx, uuid, remove); err != nil {
		return err
	}

	// One add per pair of dates, since dates apply to the whole request
	type dates struct{ starts, ends string }
	batches := map[dates][]Subject{}
	for _, m := range memberships {
		if cur, ok := currentBySubject[m.Subject]; ok && cur.Starts.Equal(m.Starts) && cur.Ends.Equal(m.Ends) {
			continue
		}

		starts, err := c.formatTime(m.Starts)
		if err != nil {
			return err
		}
		ends, err := c.formatTime(m.Ends)
		if err != nil {
			return err
		}
		key := dates{starts, ends}
		batches[key] = append(batches[key], m.Subject)
	}
	for key, batch := range batches {
		if err := c.addMembers(ctx, uuid, batch, key.starts, key.ends); err != nil {
			return err
		}
	}
	return nil
}

// Adds subjects or updates their dates, clearing dates left empty
func (c *Client) addMembers(ctx context.Context, uuid string, subjects []Subject, starts, ends string) error {
	add := map[string]any{
		"wsGroupLookup":  map[string]string{"uuid": uuid},
		"subjectLookups": subjects,
	}
	if starts != "" {
		add["enabledTime"] = starts
	}
	if ends != "" {
		add["disabledTime"] = ends
	}
	return c.post(ctx, "groups", map[string]any{"WsRestAddMemberRequest": add}, &struct{}{})
}

func (c *Client) deleteMembers(ctx context.Context, uuid string, subjects []Subject) error {
	if len(subjects) == 0 {
		return nil
	}

	req := map[string]any{
		"WsRestDeleteMemberRequest": map[string]any{
			"wsGroupLookup":  map[string]string{"uuid": uuid},
			"subjectLookups": subjects,
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
