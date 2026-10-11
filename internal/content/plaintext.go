package content

import (
	"html"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

var strict = bluemonday.StrictPolicy()

var (
	blankLines = regexp.MustCompile(`[ \t]*\n[ \t\n]*`)
	runs       = regexp.MustCompile(`[ \t]{2,}`)
)

var blockEnds = strings.NewReplacer(
	"</p>", "</p>\n", "<br>", "\n", "<br/>", "\n", "<br />", "\n",
	"</li>", "</li>\n", "</h1>", "</h1>\n", "</h2>", "</h2>\n", "</h3>", "</h3>\n",
	"</tr>", "</tr>\n", "</blockquote>", "</blockquote>\n",
)

// PlainText turns lesson HTML into readable text (for the AI tutor's
// context): tags dropped, entities decoded, whitespace tidied.
func PlainText(htmlText string) string {
	text := html.UnescapeString(strict.Sanitize(blockEnds.Replace(htmlText)))
	text = blankLines.ReplaceAllString(text, "\n")
	return strings.TrimSpace(runs.ReplaceAllString(text, " "))
}
