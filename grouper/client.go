// Package grouper is a client for the Grouper web services REST API.
package grouper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client calls one Grouper web services endpoint as one user.
type Client struct {
	url      string
	username string
	password string
	http     *http.Client
}

// NewClient returns a client for url, such as
// https://grouper.example.edu/grouper-ws/servicesRest/v2_6_000.
func NewClient(url, username, password string) *Client {
	return &Client{
		url:      strings.TrimSuffix(url, "/"),
		username: username,
		password: password,
		http:     http.DefaultClient,
	}
}

// Error is a web services call that Grouper rejected or failed.
type Error struct {
	// HTTP status code
	Status int
	// Grouper's resultCode, such as EXCEPTION, or empty when the response came from outside Grouper
	Code    string
	Message string
}

func (e *Error) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("grouper: HTTP %d: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("grouper: %s: %s", e.Code, e.Message)
}

type resultMetadata struct {
	ResultCode    string `json:"resultCode"`
	ResultMessage string `json:"resultMessage"`
}

// Posts in to a web services path and decodes a successful response into out
func (c *Client) post(ctx context.Context, path string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/"+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}

	data, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return err
	}

	switch resp.Header.Get("X-Grouper-success") {
	case "T":
		return json.Unmarshal(data, out)
	case "F":
		return grouperError(resp.StatusCode, data)
	default:
		return &Error{Status: resp.StatusCode, Message: http.StatusText(resp.StatusCode)}
	}
}

// Error from a failed response, preferring the first item's resultMetadata over the request's
func grouperError(status int, data []byte) error {
	var wrapper map[string]struct {
		ResultMetadata resultMetadata `json:"resultMetadata"`
		Results        []struct {
			ResultMetadata resultMetadata `json:"resultMetadata"`
		} `json:"results"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return &Error{Status: status, Message: string(data)}
	}

	for _, results := range wrapper {
		meta := results.ResultMetadata
		if len(results.Results) > 0 && results.Results[0].ResultMetadata.ResultCode != "" {
			meta = results.Results[0].ResultMetadata
		}
		return &Error{Status: status, Code: meta.ResultCode, Message: exceptionMessage(meta.ResultMessage)}
	}
	return &Error{Status: status, Message: string(data)}
}

// The exception from a resultMessage, without the request dump before it or anything after its first line
func exceptionMessage(message string) string {
	message, _, _ = strings.Cut(message, "\n\tat ")
	for _, marker := range []string{"Exception: ", "InvalidRequest: "} {
		if i := strings.LastIndex(message, marker); i >= 0 {
			message = message[i+len(marker):]
			break
		}
	}
	message, _, _ = strings.Cut(message, "\n")
	return strings.TrimRight(message, ", ")
}
