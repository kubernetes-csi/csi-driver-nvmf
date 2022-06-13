package client

import (
	"encoding/json"
	"net/http"

	"k8s.io/klog/v2"
)

type Response struct {
	statusCode int
	header     http.Header
	body       []byte
	err        error
}

func (r *Response) StatusCode() int {
	return r.statusCode
}

func (r *Response) Body() []byte {
	return r.body
}

func (r *Response) Err() error {
	return r.err
}

func (r *Response) Parse(m interface{}) error {
	if r.err != nil {
		return r.err
	}
	klog.Infof("Json unmarshal: %v", string(r.body))
	return json.Unmarshal(r.body, m)
}
