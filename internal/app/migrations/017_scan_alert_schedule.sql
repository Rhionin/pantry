-- One open batch alert per direction. due_at is when the quiet period ends.
-- scan_ids is a JSON array of the scans in that batch, in scan order.
-- A restart reads the row and waits until due_at, or sends immediately when
-- that time has already passed. The row is removed when the alert is sent
-- or the batch is empty, so a later start does not send the same batch again.

CREATE TABLE scan_alert_schedule (
    direction TEXT PRIMARY KEY CHECK (direction IN ('stock_in', 'stock_out')),
    due_at    TEXT NOT NULL,
    scan_ids  TEXT NOT NULL
);
