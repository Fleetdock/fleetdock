package httpapi

// paginated wraps a typed slice with pagination metadata for list responses.
func paginated[T any](items []T, total, limit, offset int) any {
	return struct {
		Items      []T        `json:"items"`
		Pagination pagination `json:"pagination"`
	}{
		Items:      items,
		Pagination: pagination{Total: total, Limit: limit, Offset: offset},
	}
}
