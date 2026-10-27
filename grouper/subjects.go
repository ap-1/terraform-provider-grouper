package grouper

import (
	"context"
	"slices"
)

// SubjectDetails is what Grouper knows about a subject.
type SubjectDetails struct {
	Subject
	Name        string
	Description string
}

// FindSubject returns the subject with subject's ID and source, or nil when there is none.
func (c *Client) FindSubject(ctx context.Context, subject Subject) (*SubjectDetails, error) {
	req := map[string]any{
		"WsRestGetSubjectsRequest": map[string]any{
			"wsSubjectLookups":     []Subject{subject},
			"includeSubjectDetail": "T",
		},
	}

	var resp struct {
		WsGetSubjectsResults struct {
			// Names of the attributeValues of each subject, in order
			SubjectAttributeNames []string `json:"subjectAttributeNames"`
			WsSubjects            []struct {
				ID              string   `json:"id"`
				SourceID        string   `json:"sourceId"`
				Name            string   `json:"name"`
				Success         string   `json:"success"`
				AttributeValues []string `json:"attributeValues"`
			} `json:"wsSubjects"`
		} `json:"WsGetSubjectsResults"`
	}
	if err := c.post(ctx, "subjects", req, &resp); err != nil {
		return nil, err
	}

	results := resp.WsGetSubjectsResults
	descriptionIndex := slices.Index(results.SubjectAttributeNames, "description")
	for _, s := range results.WsSubjects {
		if s.Success != "T" || s.ID != subject.ID || (subject.SourceID != "" && s.SourceID != subject.SourceID) {
			continue
		}

		details := &SubjectDetails{Subject: Subject{ID: s.ID, SourceID: s.SourceID}, Name: s.Name}
		if descriptionIndex >= 0 && descriptionIndex < len(s.AttributeValues) {
			details.Description = s.AttributeValues[descriptionIndex]
		}
		return details, nil
	}
	return nil, nil
}
