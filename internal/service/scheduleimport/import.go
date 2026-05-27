package scheduleimport

import (
	"context"
	"time"
)

// Import parst eine XLSX-Datei und wendet sie auf die Datenbank an.
func Import(ctx context.Context, deps Deps, file []byte, createdBy int, scope ImportScope) (*Report, error) {
	parsed, err := ParseXLSX(file)
	if err != nil {
		return nil, err
	}
	today := time.Now().In(time.Local).Format("2006-01-02")
	return Apply(ctx, deps, parsed, createdBy, today, scope)
}
