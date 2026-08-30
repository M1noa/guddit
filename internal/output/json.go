package output

import (
	"encoding/json"
	"io"

	"github.com/minoa/guddit/internal/reddit"
)

// writeJSON marshals result to pretty JSON. Listings keep their post slice; a
// withPaginator wrapper also emits the after/before cursors.
func writeJSON(result any, w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	switch v := result.(type) {
	case withPaginator:
		return enc.Encode(map[string]any{
			"posts":  v.posts,
			"after":  v.paginator.After,
			"before": v.paginator.Before,
			"count":  v.paginator.Count,
		})
	case []reddit.Post:
		return enc.Encode(map[string]any{"posts": v})
	default:
		return enc.Encode(v)
	}
}
