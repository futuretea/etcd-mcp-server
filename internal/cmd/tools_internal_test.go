package cmd

import (
	"strings"
	"testing"
)

func TestParseToolParamsRejectsTrailingJSONValue(t *testing.T) {
	_, err := parseToolParams("{}{}", "", strings.NewReader(""))
	if err == nil || !strings.Contains(err.Error(), "single JSON object") {
		t.Fatalf("parseToolParams error = %v, want trailing-value rejection", err)
	}
}
