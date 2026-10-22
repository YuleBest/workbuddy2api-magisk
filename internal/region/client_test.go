package region

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestFetchCountriesNestedJSONAndInternationalOrder(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/billing/area/get-country-code" {
			t.Errorf("path=%q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method=%q", r.Method)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["filterForbidden"] != float64(1) {
			t.Errorf("filterForbidden=%v", body["filterForbidden"])
		}
		inner := `{"code":0,"data":{"list":[` +
			`{"EnName":"Singapore","Name":"新加坡","IOS2":"SG","IOS3":"SGP","Code":"702"},` +
			`{"EnName":"United States","Name":"美国","IOS2":"US","IOS3":"USA","Code":840},` +
			`{"EnName":"Hong Kong","Name":"中国香港","IOS2":"HK","IOS3":"HKG","Code":"344"},` +
			`{"EnName":"Macao","Name":"中国澳门","IOS2":"MO","IOS3":"MAC","Code":"446"},` +
			`{"EnName":"Thailand","Name":"泰国","IOS2":"TH","IOS3":"THA","Code":"764"}` +
			`]}}`
		encoded, _ := json.Marshal(inner)
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":`+string(encoded)+`}`)
	}))
	defer server.Close()

	client := &Client{HTTPClient: server.Client(), BaseURL: server.URL}
	countries, err := client.FetchCountries(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(countries))
	for i, country := range countries {
		got[i] = country.IOS2
	}
	want := []string{"HK", "MO", "SG", "TH"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("order=%v want %v", got, want)
	}
	if countries[2].Code != "702" {
		t.Errorf("SG Code=%q want 702", countries[2].Code)
	}
}

func TestFetchCountriesFullListKeepsServerOrderAndNumericCode(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inner := `{"data":{"list":[` +
			`{"EnName":"United States","IOS2":"US","Code":840},` +
			`{"EnName":"Hong Kong","IOS2":"HK","Code":"344"}` +
			`]}}`
		encoded, _ := json.Marshal(inner)
		_, _ = io.WriteString(w, `{"code":0,"data":`+string(encoded)+`}`)
	}))
	defer server.Close()

	client := &Client{HTTPClient: server.Client(), BaseURL: server.URL}
	countries, err := client.FetchCountries(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(countries) != 2 || countries[0].IOS2 != "US" || countries[0].Code != "840" {
		t.Fatalf("countries=%+v", countries)
	}
}

func TestDetectRegionNestedJSONAndHeaders(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization=%q", got)
		}
		if got := r.Header.Get("Origin"); got != DefaultBaseURL {
			t.Errorf("Origin=%q", got)
		}
		inner, _ := json.Marshal(`{"code":0,"data":{"IOS2":"SG","enName":"Singapore"}}`)
		_, _ = w.Write([]byte(`{"code":0,"data":` + string(inner) + `}`))
	}))
	defer server.Close()

	client := &Client{HTTPClient: server.Client(), BaseURL: server.URL}
	got, err := client.DetectRegion(context.Background(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	if got.IOS2 != "SG" || got.EnName != "Singapore" {
		t.Fatalf("region=%+v", got)
	}
}

func TestSubmitRegionAttributes(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Attributes struct {
				CountryCode     []string `json:"countryCode"`
				CountryFullName []string `json:"countryFullName"`
				CountryName     []string `json:"countryName"`
			} `json:"attributes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.Attributes.CountryCode) != 1 || body.Attributes.CountryCode[0] != "702" {
			t.Errorf("countryCode=%v", body.Attributes.CountryCode)
		}
		if len(body.Attributes.CountryFullName) != 1 || body.Attributes.CountryFullName[0] != "Singapore" {
			t.Errorf("countryFullName=%v", body.Attributes.CountryFullName)
		}
		if len(body.Attributes.CountryName) != 1 || body.Attributes.CountryName[0] != "SG" {
			t.Errorf("countryName=%v", body.Attributes.CountryName)
		}
		_, _ = w.Write([]byte(`{"code":0,"msg":"ok"}`))
	}))
	defer server.Close()

	client := &Client{HTTPClient: server.Client(), BaseURL: server.URL}
	country := Country{IOS2: "SG", EnName: "Singapore", Code: "702"}
	if err := client.SubmitRegion(context.Background(), "tok", country); err != nil {
		t.Fatal(err)
	}
}

func TestActivateRegionRequiresRegionAndSucceeds(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/realms/copilot/overseas/user/register" {
			t.Errorf("path=%q", r.URL.Path)
		}
		if r.URL.Query().Get("userId") != "u1" {
			t.Errorf("userId=%q", r.URL.Query().Get("userId"))
		}
		if r.Header.Get("X-User-Id") != "u1" {
			t.Errorf("X-User-Id=%q", r.Header.Get("X-User-Id"))
		}
		if calls.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"code":500,"msg":"region required"}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":200,"msg":"register success"}`))
	}))
	defer server.Close()

	client := &Client{HTTPClient: server.Client(), BaseURL: server.URL}
	err := client.ActivateRegion(context.Background(), "tok", "u1")
	if !IsRegionRequired(err) {
		t.Fatalf("first activation err=%v want region required", err)
	}
	if err := client.ActivateRegion(context.Background(), "tok", "u1"); err != nil {
		t.Fatalf("second activation: %v", err)
	}
}

func TestTrialIdempotence(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		status      int
		body        string
		wantAlready bool
		wantErr     bool
	}{
		{name: "success", status: 200, body: `{"code":0,"msg":"ok"}`},
		{name: "already in business code", status: 200, body: `{"code":14051,"msg":"claimed"}`, wantAlready: true},
		{name: "already in nested payload", status: 200, body: `{"code":1,"data":{"bizCode":14051}}`, wantAlready: true},
		{name: "already in http error json", status: 400, body: `{"code":14051,"msg":"claimed"}`, wantAlready: true},
		{name: "already in http error text", status: 403, body: `denied 14051`, wantAlready: true},
		{name: "http failure", status: 400, body: `{"code":123,"msg":"bad"}`, wantErr: true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			client := &Client{HTTPClient: server.Client(), BaseURL: server.URL}
			already, err := client.Trial(context.Background(), "tok")
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
			if already != tc.wantAlready {
				t.Errorf("already=%v want %v", already, tc.wantAlready)
			}
		})
	}
}

func TestCompleteFlowSubmitsThenReverifies(t *testing.T) {
	t.Parallel()
	var activations atomic.Int32
	var submitted atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/realms/copilot/overseas/user/register":
			if activations.Add(1) == 1 {
				_, _ = w.Write([]byte(`{"code":500,"msg":"region required"}`))
				return
			}
			if !submitted.Load() {
				t.Error("reverify happened before submit")
			}
			_, _ = w.Write([]byte(`{"code":200}`))
		case "/console/login/account":
			submitted.Store(true)
			_, _ = w.Write([]byte(`{"code":0}`))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	client := &Client{HTTPClient: server.Client(), BaseURL: server.URL}
	pick := Country{IOS2: "SG", EnName: "Singapore", Code: "702"}
	if err := client.CompleteFlow(context.Background(), "tok", "u1", &pick, true); err != nil {
		t.Fatal(err)
	}
	if activations.Load() != 2 {
		t.Errorf("activation calls=%d want 2", activations.Load())
	}
}

func TestInteractiveMenuDetectedMarkAndCancel(t *testing.T) {
	t.Parallel()
	countries := []Country{
		{IOS2: "HK", EnName: "Hong Kong", Code: "344"},
		{IOS2: "SG", EnName: "Singapore", Code: "702"},
	}
	var out strings.Builder
	picked := InteractiveMenu(countries, "SG", "", strings.NewReader("\n"), &out)
	if picked != nil {
		t.Fatalf("picked=%+v want nil", picked)
	}
	if !strings.Contains(out.String(), "Singapore  ◀ 检测到当前地区") {
		t.Errorf("menu output missing detected marker:\n%s", out.String())
	}
}
