-- Deployment status events were written with `'Deployment '+lower(status)`.
-- SQLite's `+` is arithmetic, so the expression evaluated to 0 and every
-- affected row stored the message "0". Rebuild the message from the status
-- already on the row.
--
-- The guard is deliberately narrow: only deployment events whose message is
-- exactly "0" can have come from that expression.
UPDATE platform_events
   SET message = 'Deployment ' || lower(status)
 WHERE event_type = 'deployment'
   AND message = '0'
   AND status <> '';
