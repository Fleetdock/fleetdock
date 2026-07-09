package postgres

import "github.com/google/uuid"

// idArray returns a never-nil uuid slice so an `= ANY($n)` filter with no ids
// matches zero rows (rather than erroring on a nil encode).
func idArray(ids []uuid.UUID) []uuid.UUID {
	if ids == nil {
		return []uuid.UUID{}
	}
	return ids
}
