package grouper

import "context"

// Stem is a Grouper folder.
type Stem struct {
	// Full name, such as org:dept:apps
	Name string `json:"name"`
	// Last segment of DisplayName
	DisplayExtension string `json:"displayExtension"`
	// Full name with each segment's display name
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	UUID        string `json:"uuid"`
}

// FindStem returns the folder with the given full name, or nil when there is none.
func (c *Client) FindStem(ctx context.Context, name string) (*Stem, error) {
	req := map[string]any{
		"WsRestFindStemsRequest": map[string]any{
			"wsStemQueryFilter": map[string]string{
				"stemName":            name,
				"stemQueryFilterType": "FIND_BY_STEM_NAME",
			},
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
