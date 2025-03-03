package traefik_outgoing_oauth2_cc

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

//goland:noinspection SpellCheckingInspection
func TestSimple(t *testing.T) {
	baselineTest(t, "user", "pass", "Basic dXNlcjpwYXNz")
}

//goland:noinspection SpellCheckingInspection
func TestSpecialChars(t *testing.T) {
	// user%2F%3D%3F%26:pass%2F%3D%3F%26  (url encoded /=?&)
	baselineTest(t, "user/=?&", "pass/=?&", "Basic dXNlciUyRiUzRCUzRiUyNjpwYXNzJTJGJTNEJTNGJTI2")
}

//goland:noinspection GoUnhandledErrorResult,SpellCheckingInspection
func baselineTest(t *testing.T, user string, pass string, authHeader string) {
	authCallIndex := 0
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != authHeader {
			t.Errorf("auth grant Authorization header mismatch: expected '%s' != '%s' actual", authHeader, req.Header.Get("Authorization"))
			rw.WriteHeader(http.StatusForbidden)
			rw.Write([]byte("Forbidden (wrong auth header)"))
		} else {
			rw.Write([]byte(fmt.Sprintf(`{"access_token": "test_token%d", "expires_in": 3600}`, authCallIndex)))
			authCallIndex += 1
		}
	}))
	defer server.Close()

	tmpdir := t.TempDir()
	passfile := tmpdir + "/passfile"
	cfg := CreateConfig()
	cfg.AuthGrantRequest.User = user
	cfg.AuthGrantRequest.Pass = "~file~" + passfile
	cfg.AuthGrantRequest.URL = server.URL + "/auth"
	cfg.AuthGrantRequest.Headers = []Header{
		{Name: "header1", Value: "value1"},
	}
	err := os.WriteFile(passfile, []byte(pass), 0600)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {})
	handler, err := New(ctx, next, cfg, "test")
	if err != nil {
		t.Fatal(err)
	}

	testCall(t, ctx, server, handler)
	testCall(t, ctx, server, handler) // ensure the token is reused
}

func testCall(t *testing.T, ctx context.Context, server *httptest.Server, handler http.Handler) {
	t.Helper()
	recorder := httptest.NewRecorder()
	req, err2 := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/data", nil)
	if err2 != nil {
		t.Fatal(err2)
	}
	handler.ServeHTTP(recorder, req)
	if recorder.Result().StatusCode != http.StatusOK {
		t.Errorf("http status mismatch: expected %d != %d actual", http.StatusOK, recorder.Result().StatusCode)
	}
	const expDataAuthHeader = "Bearer test_token0"
	if req.Header.Get("Authorization") != expDataAuthHeader {
		t.Errorf("data request Authorization header mismatch: expected '%s' != '%s' actual", expDataAuthHeader, req.Header.Get("Authorization"))
	}
}

//goland:noinspection GoUnhandledErrorResult
func TestFromFlexibleField(t *testing.T) {
	tmpdir := t.TempDir()
	tests := []struct {
		name      string
		input     string
		setup     func()
		expected  string
		expectErr string
	}{
		{
			name:     "Direct value",
			input:    "direct_value",
			expected: "direct_value",
		},
		{
			name:     "Explicit Direct value",
			input:    "~direct~direct_value",
			expected: "direct_value",
		},
		{
			name:     "Leading tilde",
			input:    "~direct~~`",
			expected: "~`",
		},
		{
			name:  "File value",
			input: "~file~" + tmpdir + "/testfile.txt",
			setup: func() {
				os.WriteFile(tmpdir + "/testfile.txt", []byte("file_value"), 0644)
			},
			expected: "file_value",
		},
		{
			name:  "Env value",
			input: "~env~TEST_ENV",
			setup: func() {
				os.Setenv("TEST_ENV", "env_value")
			},
			expected: "env_value",
		},
		{
			name:  "Base64 env value",
			input: "~base64~env~TEST_ENV_BASE64",
			setup: func() {
				os.Setenv("TEST_ENV_BASE64", "ZW52X3ZhbHVl")
			},
			expected: "env_value",
		},
		{
			name:      "Invalid Base64",
			input:     "~base64~direct~aa`",
			expectErr: "illegal base64 data at input byte",
		},
		{
			name:      "Base64 UrlEncoding",
			input:     "~base64~direct~YWE",
			expected:  "aa",
		},
		{
			name:      "Base64 UrlEncoding Underscore",
			input:     "~base64~direct~Pz8_",
			expected:  "???",
		},
		{
			name:      "Base64 StdEncoding Padding",
			input:     "~base64~direct~YWE=",
			expected:  "aa",
		},
		{
			name:      "Base64 StdEncoding Slash",
			input:     "~base64~direct~Pz8/",
			expected:  "???",
		},
		{
			name:      "Non-existing env variable",
			input:     "~env~does-not-exist",
			expectErr: "environment variable 'does-not-exist' not found",
		},
		{
			name:      "Non-existing file",
			input:     "~file~does-not-exist",
			expectErr: "no such file or directory",
		},
		{
			name:      "Invalid Base64 option without source specification",
			input:     "~base64~format",
			expectErr: "invalid flexible fieldValue format",
		},
		{
			name:      "Invalid format",
			input:     "~invalid~format",
			expectErr: "unknown source: invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup()
			}
			result, err := fromFlexibleField(tt.name, tt.input)
			if tt.expectErr != "" {
				if err == nil {
					t.Errorf("expected error, got success")
				} else if !strings.Contains(err.Error(), tt.expectErr) {
					t.Errorf("expected error: %v, got error: %v", tt.expectErr, err)
				}
			} else {
				if err != nil {
					t.Errorf("expected: %v, got: error %v", tt.expected, err)
				} else if result != tt.expected {
					t.Errorf("expected: %v, got: %v", tt.expected, result)
				}
			}
		})
	}
}
