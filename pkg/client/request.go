package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"

	"k8s.io/klog/v2"
)

type Request struct {
	client *Client
	verb   string
	header http.Header
	param  url.Values
	body   io.Reader

	action     string
	pathPrefix string
	err        error
	ctx        context.Context
}

func NewRequest(client *Client) *Request {
	return &Request{
		client: client,
	}
}

func (r *Request) Verb(verb string) *Request {
	r.verb = verb
	return r
}

func (r *Request) SetHeader(key, value string) *Request {
	if r.header == nil {
		r.header = http.Header{}
	}

	r.header.Add(key, value)
	return r
}

func (r *Request) SetParam(key, value string) *Request {
	if r.param == nil {
		r.param = url.Values{}
	}

	r.param.Add(key, value)
	return r
}

func (r *Request) Body(obj interface{}) *Request {
	switch t := obj.(type) {
	case []byte:
		r.body = bytes.NewReader(t)
	case io.Reader:
		r.body = t
	default:
		r.err = fmt.Errorf("unknown type used for body: %+v", obj)
	}
	return r
}

func (r *Request) Action(action string) *Request {
	r.action = action
	return r
}

func (r *Request) Prefix(prefix string) *Request {
	r.pathPrefix = prefix
	return r
}

func (r *Request) Context(context context.Context) *Request {
	r.ctx = context
	return r
}

func (r *Request) URL() *url.URL {
	parts := []string{r.pathPrefix}
	if r.action != "" {
		parts = append(parts, r.action)
	}

	result := *r.client.baseURL
	result.Path = path.Join(parts...)

	query := url.Values{}
	for key, values := range r.param {
		for _, value := range values {
			query.Add(key, value)
		}
	}
	result.RawQuery = query.Encode()

	return &result
}

func (r *Request) request() (*http.Response, error) {
	client := r.client.httpClient
	if client == nil {
		klog.Info("Use Default HTTP Client.")
		client = http.DefaultClient
	}

	url := r.URL().String()
	klog.Infof("Request: %s", url)

	req, err := http.NewRequest(r.verb, url, r.body)
	if err != nil {
		klog.Errorf("Request: %s error: %s", url, err)
		return nil, err
	}

	if r.ctx != nil {
		req = req.WithContext(req.Context())
	}
	req.Header = r.header

	return client.Do(req)
}

func (r *Request) Do() *Response {
	if r.err != nil {
		return &Response{
			err: r.err,
		}
	}

	rsp, err := r.request()
	if err != nil {
		return &Response{
			err: r.err,
		}
	}

	body, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		return &Response{
			err: r.err,
		}
	}

	return &Response{
		statusCode: rsp.StatusCode,
		header:     rsp.Header,
		body:       body,
	}
}
