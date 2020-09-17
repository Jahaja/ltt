package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"ltt/loadtest"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
)

type httpClientContextKeyType int

var httpClientContextKey httpClientContextKeyType

func NewHTTPClient(ctx context.Context, baseURI string) *HTTPClient {
	jar, err := cookiejar.New(&cookiejar.Options{})
	if err != nil {
		log.Fatalf("failed to create cookie jar: %w", err)
	}

	lt := loadtest.FromContext(ctx)
	std_client := &http.Client{
		Timeout: lt.Config.RequestTimeout,
		Jar:     jar,
	}
	client := &HTTPClient{
		std:              std_client,
		baseURI:          baseURI,
		user:             loadtest.UserFromContext(ctx),
		Headers:          make(http.Header),
		ErrorOnErrorCode: true,
	}

	return client
}

func NewHTTPClientContext(ctx context.Context, client *HTTPClient) context.Context {
	return context.WithValue(ctx, httpClientContextKey, client)
}

func HTTPFromContext(ctx context.Context) *HTTPClient {
	if c, ok := ctx.Value(httpClientContextKey).(*HTTPClient); ok {
		return c
	}

	return nil
}

type HTTPClient struct {
	std *http.Client
	// Base uri to append the path to, e.g. "http://localhost/"
	baseURI string
	// The owning User of this http client
	user loadtest.User
	// These headers will be set in all requests
	Headers http.Header
	// If true, 4xx-5xx status code will return an error, defaults to true
	ErrorOnErrorCode bool
}

type HTTPResponse struct {
	StatusCode  int
	Body        []byte
	RawResponse *http.Response
}

func (resp *HTTPResponse) JSON(out interface{}) error {
	return json.Unmarshal(resp.Body, out)
}

func (c *HTTPClient) getUrl(path string) string {
	return fmt.Sprint(strings.TrimRight(c.baseURI, "/"), path)
}

func (c *HTTPClient) handleResponse(method string, path string, std_resp *http.Response) (*HTTPResponse, error) {
	response_body, err := ioutil.ReadAll(std_resp.Body)
	if err != nil {
		return nil, err
	}

	if std_resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("error status code %d: %s", std_resp.StatusCode, string(response_body))
	}

	resp := &HTTPResponse{
		StatusCode:  std_resp.StatusCode,
		Body:        response_body,
		RawResponse: std_resp,
	}

	log.Printf("HTTPClient(user %d): response for %s %s: %d (%d bytes)\n",
		c.user.ID(), method, path, resp.StatusCode, len(resp.Body))

	return resp, nil
}

func (c *HTTPClient) Request(method string, path string, body []byte) (*HTTPResponse, error) {
	log.Printf("HTTPClient(user %d): requesting %s %s\n", c.user.ID(), method, path)

	std_req, err := http.NewRequest(method, c.getUrl(path), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	// Set the default headers
	for k, v := range c.Headers {
		std_req.Header[k] = v
	}

	std_resp, err := c.std.Do(std_req)
	if err != nil {
		return nil, err
	}

	return c.handleResponse(method, path, std_resp)
}

func (c *HTTPClient) Get(path string) (*HTTPResponse, error) {
	return c.Request(http.MethodGet, path, nil)
}

func (c *HTTPClient) Options(path string) (*HTTPResponse, error) {
	return c.Request(http.MethodOptions, path, nil)
}

func (c *HTTPClient) Head(path string) (*HTTPResponse, error) {
	return c.Request(http.MethodHead, path, nil)
}

func (c *HTTPClient) Delete(path string) (*HTTPResponse, error) {
	return c.Request(http.MethodDelete, path, nil)
}

func (c *HTTPClient) Post(path string, body []byte) (*HTTPResponse, error) {
	return c.Request(http.MethodPost, path, body)
}

func (c *HTTPClient) PostForm(path string, data url.Values) (*HTTPResponse, error) {
	log.Printf("HTTPClient(user %d): requesting form POST %s\n", c.user.ID(), path)

	std_resp, err := c.std.PostForm(c.getUrl(path), data)
	if err != nil {
		return nil, err
	}

	return c.handleResponse(http.MethodPost, path, std_resp)
}

func (c *HTTPClient) Patch(path string, body []byte) (*HTTPResponse, error) {
	return c.Request(http.MethodPatch, path, body)
}

func (c *HTTPClient) Put(path string, body []byte) (*HTTPResponse, error) {
	return c.Request(http.MethodPut, path, body)
}
