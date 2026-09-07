package markdown

import (
	"fmt"
	"strings"
)

// RenderPage renders the canonical schema for a newly created memory page.
func RenderPage(pageID, title, body string) []byte {
	body = strings.TrimSuffix(strings.ReplaceAll(body, "\r\n", "\n"), "\n") + "\n"
	return []byte(fmt.Sprintf("# %s\n<!-- @id: %s -->\n\n## Overview\n<!-- @id: %s-overview -->\n%s", title, pageID, pageID, body))
}
