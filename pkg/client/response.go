package client

import "net/http"

type Response struct {
	statusCode int
	header     http.Header
	body       []byte
	err        error
}
