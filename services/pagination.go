package services

import "gorm.io/gorm"

// queryPage applies the common count/order/limit/offset sequence used by the
// administrative list endpoints. Keeping the query construction in one place
// prevents individual services from drifting on page-size limits or offsets.
func queryPage[T any](db *gorm.DB, params map[string]string, maxSize int, order string) ([]T, int64, error) {
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page := ParsePage(params, maxSize)
	query := db
	if order != "" {
		query = query.Order(order)
	}
	var items []T
	err := query.Limit(page.Size).Offset((page.Page - 1) * page.Size).Find(&items).Error
	return items, total, err
}

// queryPageCursor is the cursor-aware variant used by large, append-heavy
// tables such as events, sessions and access logs. It retains the total count
// in the API response while avoiding a deep offset when a cursor is supplied.
func queryPageCursor[T any](db *gorm.DB, params map[string]string, maxSize int, timeColumn, order string) ([]T, int64, error) {
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page := ParsePage(params, maxSize)
	query, cursor := ApplyBeforeCursor(db, params, timeColumn)
	offset := (page.Page - 1) * page.Size
	if cursor {
		offset = 0
	}
	var items []T
	err := query.Order(order).Limit(page.Size).Offset(offset).Find(&items).Error
	return items, total, err
}
