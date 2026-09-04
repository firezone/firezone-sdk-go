package firezone

// ListOptions controls pagination for List methods. The zero value
// requests the API's default page (limit 50).
type ListOptions struct {
	// Limit is the maximum number of items to return. The API clamps
	// this to the range [1, 100]; zero means "use the API default" (50).
	Limit int
	// PageCursor requests the page following (or preceding, when used
	// with a PrevPage cursor) the one it was returned from. Leave empty
	// to request the first page.
	PageCursor string
}

// PageMetadata describes a page of results returned by a List method.
type PageMetadata struct {
	// Count is the total number of items across all pages.
	Count int
	// Limit is the page size that was actually applied.
	Limit int
	// NextPage is the cursor for the next page, or "" if this is the
	// last page.
	NextPage string
	// PrevPage is the cursor for the previous page, or "" if this is
	// the first page.
	PrevPage string
}

// Page is one page of results from a List method.
type Page[T any] struct {
	Data     []T
	Metadata PageMetadata
}

// pageMetadataBody mirrors the "metadata" object in a list response.
type pageMetadataBody struct {
	Count    int    `json:"count"`
	Limit    int    `json:"limit"`
	NextPage string `json:"next_page"`
	PrevPage string `json:"prev_page"`
}

func (b pageMetadataBody) toMetadata() PageMetadata {
	return PageMetadata(b)
}
