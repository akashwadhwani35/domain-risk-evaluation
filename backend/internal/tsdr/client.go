package tsdr

import "context"

// Client is a placeholder for future TSDR integration.
type Client struct{}

// NewClient returns a stub client.
func NewClient() *Client {
	return &Client{}
}

// GetStatus is a stub implementation returning nil until live integration is available.
func (c *Client) GetStatus(ctx context.Context, caseID string) (interface{}, error) {
	return nil, nil
}
