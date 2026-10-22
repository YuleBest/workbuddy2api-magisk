// Package region implements the WorkBuddy global-account registration flow.
package region

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const DefaultBaseURL = "https://www.workbuddy.ai"

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

// InternationalCodes is the region list shown by the international web client.
var InternationalCodes = []string{"HK", "MO", "SG", "TH", "PH", "MY", "ID"}

// Country is one entry returned by the region service.
type Country struct {
	EnName string
	Name   string
	IOS2   string
	IOS3   string
	Code   string
}

// UnmarshalJSON accepts both string and numeric Code values.
func (c *Country) UnmarshalJSON(raw []byte) error {
	var doc struct {
		EnName string          `json:"EnName"`
		Name   string          `json:"Name"`
		IOS2   string          `json:"IOS2"`
		IOS3   string          `json:"IOS3"`
		Code   json.RawMessage `json:"Code"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return err
	}
	code, err := jsonScalarString(doc.Code)
	if err != nil {
		return fmt.Errorf("country Code: %w", err)
	}
	*c = Country{
		EnName: doc.EnName,
		Name:   doc.Name,
		IOS2:   doc.IOS2,
		IOS3:   doc.IOS3,
		Code:   code,
	}
	return nil
}

// Region is the subset returned by the current-area endpoint.
type Region struct {
	IOS2   string `json:"IOS2"`
	EnName string `json:"enName"`
}

// RegionRequiredError reports that register needs a registration region first.
type RegionRequiredError struct {
	Msg string
}

func (e *RegionRequiredError) Error() string { return e.Msg }

// IsRegionRequired reports whether err means a region must be submitted.
func IsRegionRequired(err error) bool {
	var target *RegionRequiredError
	return errors.As(err, &target)
}

// HTTPError preserves an unsuccessful HTTP response for CLI diagnostics.
type HTTPError struct {
	StatusCode int
	Body       []byte
}

func (e *HTTPError) Error() string {
	body := strings.TrimSpace(string(e.Body))
	if body == "" {
		return fmt.Sprintf("http %d", e.StatusCode)
	}
	if len(body) > 120 {
		body = body[:120]
	}
	return fmt.Sprintf("http %d: %s", e.StatusCode, body)
}

// Client talks to the global billing and registration endpoints.
type Client struct {
	HTTPClient *http.Client
	BaseURL    string
}

// NewClient returns a client using the production base and a bounded timeout.
func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &Client{HTTPClient: httpClient, BaseURL: DefaultBaseURL}
}

func (c *Client) client() *http.Client {
	if c != nil && c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func (c *Client) baseURL() string {
	if c != nil && strings.TrimSpace(c.BaseURL) != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return DefaultBaseURL
}

func headers(token string, extra map[string]string) http.Header {
	h := make(http.Header)
	h.Set("User-Agent", userAgent)
	h.Set("Accept", "application/json, text/plain, */*")
	h.Set("Content-Type", "application/json")
	h.Set("Origin", DefaultBaseURL)
	h.Set("Referer", DefaultBaseURL+"/")
	if token != "" {
		h.Set("Authorization", "Bearer "+token)
	}
	for k, v := range extra {
		h.Set(k, v)
	}
	return h
}

func (c *Client) do(
	ctx context.Context,
	method, path, token string,
	body any,
	extra map[string]string,
) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL()+path, reader)
	if err != nil {
		return 0, nil, err
	}
	req.Header = headers(token, extra)
	resp, err := c.client().Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, raw, nil
}

func checkHTTP(status int, raw []byte) error {
	if status >= 200 && status < 300 {
		return nil
	}
	return &HTTPError{StatusCode: status, Body: raw}
}

func decodeEnvelope(raw []byte) (int, string, json.RawMessage, error) {
	var env struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = []byte("{}")
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return 0, "", nil, err
	}
	return env.Code, env.Msg, env.Data, nil
}

func decodeNested(raw json.RawMessage, dst any) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil
	}
	if raw[0] == '"' {
		var encoded string
		if err := json.Unmarshal(raw, &encoded); err != nil {
			return err
		}
		raw = []byte(encoded)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	return json.Unmarshal(raw, dst)
}

func envelopeMessage(code int, msg string) string {
	if strings.TrimSpace(msg) != "" {
		return msg
	}
	return fmt.Sprintf("code=%d", code)
}

// FetchCountries returns the selectable country list. When intlOnly is true,
// the result follows the international web client's fixed display order.
func (c *Client) FetchCountries(ctx context.Context, intlOnly bool) ([]Country, error) {
	status, raw, err := c.do(ctx, http.MethodPost, "/billing/area/get-country-code", "", map[string]any{
		"filterForbidden": 1,
	}, nil)
	if err != nil {
		return nil, err
	}
	if err := checkHTTP(status, raw); err != nil {
		return nil, err
	}
	code, msg, data, err := decodeEnvelope(raw)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, errors.New(envelopeMessage(code, msg))
	}
	var inner struct {
		Data struct {
			List []Country `json:"list"`
		} `json:"data"`
	}
	if err := decodeNested(data, &inner); err != nil {
		return nil, err
	}
	if !intlOnly {
		return inner.Data.List, nil
	}
	byCode := make(map[string]Country, len(inner.Data.List))
	for _, country := range inner.Data.List {
		byCode[country.IOS2] = country
	}
	out := make([]Country, 0, len(InternationalCodes))
	for _, code := range InternationalCodes {
		if country, ok := byCode[code]; ok {
			out = append(out, country)
		}
	}
	return out, nil
}

// DetectRegion asks the service which region is currently attached to the token.
func (c *Client) DetectRegion(ctx context.Context, token string) (Region, error) {
	status, raw, err := c.do(ctx, http.MethodPost, "/billing/area/get-user-area-info", token, map[string]any{
		"action": "getUserAreaInfo",
	}, nil)
	if err != nil {
		return Region{}, err
	}
	if err := checkHTTP(status, raw); err != nil {
		return Region{}, err
	}
	code, msg, data, err := decodeEnvelope(raw)
	if err != nil {
		return Region{}, err
	}
	if code != 0 {
		return Region{}, errors.New(envelopeMessage(code, msg))
	}
	var inner struct {
		Data Region `json:"data"`
	}
	if err := decodeNested(data, &inner); err != nil {
		return Region{}, err
	}
	return inner.Data, nil
}

// SubmitRegion writes the selected country to the account attributes.
func (c *Client) SubmitRegion(ctx context.Context, token string, country Country) error {
	body := map[string]any{
		"attributes": map[string]any{
			"countryCode":     []string{country.Code},
			"countryFullName": []string{country.EnName},
			"countryName":     []string{country.IOS2},
		},
	}
	status, raw, err := c.do(ctx, http.MethodPost, "/console/login/account", token, body, nil)
	if err != nil {
		return err
	}
	if err := checkHTTP(status, raw); err != nil {
		return err
	}
	code, msg, _, err := decodeEnvelope(raw)
	if err != nil {
		return err
	}
	if code != 0 {
		return errors.New(envelopeMessage(code, msg))
	}
	return nil
}

// ActivateRegion queries registration. A RegionRequiredError means the caller
// should submit a region and verify again.
func (c *Client) ActivateRegion(ctx context.Context, token, uid string) error {
	status, raw, err := c.do(
		ctx,
		http.MethodGet,
		"/auth/realms/copilot/overseas/user/register?userId="+uid,
		token,
		nil,
		map[string]string{"X-User-Id": uid},
	)
	if err != nil {
		return err
	}
	if err := checkHTTP(status, raw); err != nil {
		return err
	}
	code, msg, _, err := decodeEnvelope(raw)
	if err != nil {
		return err
	}
	if code == 200 {
		return nil
	}
	if code == 500 || strings.Contains(strings.ToLower(msg), "region required") {
		return &RegionRequiredError{Msg: envelopeMessage(code, msg)}
	}
	return errors.New(envelopeMessage(code, msg))
}

// Trial claims the one-time global trial. The upstream 14051 code means the
// benefit was already claimed and is treated as an idempotent success.
func (c *Client) Trial(ctx context.Context, token string) (already bool, err error) {
	status, raw, err := c.do(ctx, http.MethodPost, "/billing/ide/trial", token, map[string]any{}, nil)
	if err != nil {
		return false, err
	}
	code, msg, _, parseErr := decodeEnvelope(raw)
	if status < 200 || status >= 300 {
		if strings.Contains(string(raw), "14051") || bodyContainsCode(raw, "14051") {
			return true, nil
		}
		if parseErr == nil && msg != "" {
			return false, errors.New(msg)
		}
		return false, (&HTTPError{StatusCode: status, Body: raw})
	}
	if parseErr != nil {
		return false, parseErr
	}
	if code == 0 {
		return false, nil
	}
	if code == 14051 || bodyContainsCode(raw, "14051") {
		return true, nil
	}
	return false, errors.New(envelopeMessage(code, msg))
}

func bodyContainsCode(raw []byte, wanted string) bool {
	var value any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&value); err != nil {
		return false
	}
	var walk func(any) bool
	walk = func(v any) bool {
		switch x := v.(type) {
		case map[string]any:
			for _, child := range x {
				if walk(child) {
					return true
				}
			}
		case []any:
			for _, child := range x {
				if walk(child) {
					return true
				}
			}
		case string:
			return x == wanted
		case json.Number:
			return x.String() == wanted
		case float64:
			return strconv.FormatFloat(x, 'f', -1, 64) == wanted
		}
		return false
	}
	return walk(value)
}

// CompleteFlow checks registration, submits a region when required, and verifies
// the result again. Set callRegister=false when the caller has already observed a
// RegionRequiredError and only needs submit-and-reverify.
func (c *Client) CompleteFlow(ctx context.Context, token, uid string, pick *Country, callRegister bool) error {
	if callRegister {
		err := c.ActivateRegion(ctx, token, uid)
		if err == nil {
			return nil
		}
		if !IsRegionRequired(err) {
			return fmt.Errorf("register 失败: %w", err)
		}
	}
	if pick == nil {
		return errors.New("需完善注册地区，但未提供选择")
	}
	if err := c.SubmitRegion(ctx, token, *pick); err != nil {
		return fmt.Errorf("提交地区失败: %w", err)
	}
	if err := c.ActivateRegion(ctx, token, uid); err != nil {
		return fmt.Errorf("提交地区后 register 仍失败: %w (needs_region=%t)", err, IsRegionRequired(err))
	}
	return nil
}

func jsonScalarString(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", nil
	}
	if raw[0] == '"' {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", err
		}
		return value, nil
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err != nil {
		return "", err
	}
	return number.String(), nil
}
