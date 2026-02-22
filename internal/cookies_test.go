package internal

import (
	"os"
	"strings"
	"testing"
)

func TestJsonCookiesToNetscape(t *testing.T) {
	jsonContent := `[
		{
			"domain": ".instagram.com",
			"expirationDate": 1806320300.714377,
			"hostOnly": false,
			"httpOnly": false,
			"name": "csrftoken",
			"path": "/",
			"secure": true,
			"session": false,
			"value": "abc123"
		},
		{
			"domain": "www.instagram.com",
			"hostOnly": true,
			"httpOnly": true,
			"name": "sessionid",
			"path": "/",
			"secure": true,
			"session": true,
			"value": "sess456"
		}
	]`

	f, err := os.CreateTemp("", "cookies-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(jsonContent)
	f.Close()

	netscapePath, err := jsonCookiesToNetscape(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer os.Remove(netscapePath)

	data, err := os.ReadFile(netscapePath)
	if err != nil {
		t.Fatalf("cannot read netscape file: %v", err)
	}
	content := string(data)

	// Non-session cookie: domain starts with ".", include_subdomains=TRUE, expiry set
	if !strings.Contains(content, ".instagram.com\tTRUE\t/\tTRUE\t1806320300\tcsrftoken\tabc123") {
		t.Errorf("missing or wrong non-session cookie line; got:\n%s", content)
	}

	// Session cookie: hostOnly domain (no dot), include_subdomains=FALSE, expiry=0
	if !strings.Contains(content, "www.instagram.com\tFALSE\t/\tTRUE\t0\tsessionid\tsess456") {
		t.Errorf("missing or wrong session cookie line; got:\n%s", content)
	}
}

func TestJsonCookiesToNetscape_InvalidJSON(t *testing.T) {
	f, err := os.CreateTemp("", "cookies-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString("not json")
	f.Close()

	_, err = jsonCookiesToNetscape(f.Name())
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestJsonCookiesToNetscape_MissingFile(t *testing.T) {
	_, err := jsonCookiesToNetscape("/nonexistent/path/cookies.json")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}
