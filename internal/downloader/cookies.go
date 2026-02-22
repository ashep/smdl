package downloader

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// jsonCookie matches the JSON format exported by browser cookie extensions.
type jsonCookie struct {
	Domain         string  `json:"domain"`
	ExpirationDate float64 `json:"expirationDate"`
	HostOnly       bool    `json:"hostOnly"`
	HTTPOnly       bool    `json:"httpOnly"`
	Name           string  `json:"name"`
	Path           string  `json:"path"`
	Secure         bool    `json:"secure"`
	Session        bool    `json:"session"`
	Value          string  `json:"value"`
}

// jsonCookiesToNetscape converts a JSON cookie list to a Netscape cookies file
// written to a temporary file, returning its path. The caller must delete it.
func (d *Downloader) jsonCookiesToNetscape(jsonContent string) (string, error) {
	var cookies []jsonCookie
	if err := json.Unmarshal([]byte(jsonContent), &cookies); err != nil {
		return "", fmt.Errorf("parse cookies JSON: %w", err)
	}

	var buf bytes.Buffer
	buf.WriteString("# Netscape HTTP Cookie File\n")
	for _, c := range cookies {
		includeSubdomains := "FALSE"
		if strings.HasPrefix(c.Domain, ".") {
			includeSubdomains = "TRUE"
		}
		secure := "FALSE"
		if c.Secure {
			secure = "TRUE"
		}
		expiry := int64(0)
		if !c.Session {
			expiry = int64(c.ExpirationDate)
		}
		fmt.Fprintf(&buf, "%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
			c.Domain, includeSubdomains, c.Path, secure, expiry, c.Name, c.Value)
	}

	tmp, err := os.CreateTemp("", "smdl-cookies-*.txt")
	if err != nil {
		return "", fmt.Errorf("create temp cookies file: %w", err)
	}
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", fmt.Errorf("write temp cookies file: %w", err)
	}
	tmp.Close()

	return tmp.Name(), nil
}
