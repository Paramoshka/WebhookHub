package storage

import (
	"context"

	"webhookhub/internal/model"
)

// StreamWebhooks yields every webhook matching filter in sort order, one row
// at a time. The single statement gives the export a consistent snapshot, and
// the caller's context bounds the work. A yield error stops the stream.
func (d *DB) StreamWebhooks(ctx context.Context, filter WebhookFilter, yield func(model.Webhook) error) error {
	query := d.applyWebhookFilter(d.conn.WithContext(ctx).Model(&model.Webhook{}), filter)
	query = d.applyWebhookSort(query, filter.Sort)
	rows, err := query.Rows()
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var hook model.Webhook
		if err := d.conn.ScanRows(rows, &hook); err != nil {
			return err
		}
		if err := yield(hook); err != nil {
			return err
		}
	}
	return rows.Err()
}
