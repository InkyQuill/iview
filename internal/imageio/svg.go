package imageio

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"os"
	"regexp"
	"strings"
)

var (
	errSVGExternalReference = errors.New("svg contains external resource reference")
	svgEventHandlerPattern  = regexp.MustCompile(`(?i)\son[a-z0-9_.:-]+\s*=`)
	svgResourceAttrPattern  = regexp.MustCompile(`(?i)(?:^|[\s<])(href|xlink:href|src)\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)`)
	svgDTDPattern           = regexp.MustCompile(`(?i)<![^>]*\b(system|public)\b`)
)

func validateSVGStaticSafety(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read svg: %w", err)
	}
	if !isSVGStaticSafe(content) {
		return errSVGExternalReference
	}
	return nil
}

func isSVGStaticSafe(content []byte) bool {
	decodedContent := []byte(html.UnescapeString(string(content)))
	lower := bytes.ToLower(decodedContent)
	if bytes.Contains(lower, []byte("<script")) ||
		bytes.Contains(lower, []byte("<!doctype")) ||
		bytes.Contains(lower, []byte("<!entity")) ||
		bytes.Contains(lower, []byte("url(")) ||
		bytes.Contains(lower, []byte("@import")) ||
		bytes.Contains(decodedContent, []byte(`\`)) ||
		svgDTDPattern.Match(decodedContent) {
		return false
	}
	if svgEventHandlerPattern.Match(decodedContent) {
		return false
	}

	for _, match := range svgResourceAttrPattern.FindAllSubmatch(decodedContent, -1) {
		if len(match) < 3 {
			continue
		}
		if isUnsafeSVGResourceValue(string(match[2])) {
			return false
		}
	}

	return true
}

func isUnsafeSVGResourceValue(value string) bool {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	value = strings.TrimSpace(value)
	value = html.UnescapeString(value)
	value = strings.TrimSpace(value)

	return value != "" && !strings.HasPrefix(value, "#")
}
