package client

import (
	"net/http"
	"net/url"
)

type Client struct {
	httpClient *http.Client
	baseURL    *url.URL
}

func NewClient(baseURLStr string) (*Client, error) {
	baseURL, err := url.Parse(baseURLStr)
	if err != nil {
		return nil, err
	}

	httpClient := &http.Client{}

	return &Client{
		httpClient: httpClient,
		baseURL:    baseURL,
	}, nil
}

func (c *Client) verb(verb string) *Request {
	return NewRequest(c).Verb(verb)
}

func (c *Client) Post() *Request {
	return c.verb("POST")
}

func (c *Client) Get() *Request {
	return c.verb("GET")
}

func (c *Client) Delete() *Request {
	return c.verb("DELETE")
}

func (c *Client) Raw() *http.Client {
	return c.httpClient
}
