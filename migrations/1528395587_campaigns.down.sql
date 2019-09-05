BEGIN;

DROP TABLE IF EXISTS labels_objects;
DROP TABLE IF EXISTS labels;

-----------------

ALTER TABLE events DROP COLUMN thread_diagnostic_edge_id;
DROP TABLE IF EXISTS thread_diagnostic_edges;

-----------------

ALTER TABLE events DROP COLUMN rule_id;

DROP TABLE IF EXISTS rules;

ALTER TABLE campaigns DROP COLUMN due_date;
ALTER TABLE campaigns DROP COLUMN start_date;
ALTER TABLE campaigns DROP COLUMN is_draft;
ALTER TABLE campaigns DROP COLUMN template_context;
ALTER TABLE campaigns DROP COLUMN template_id;

ALTER TABLE threads DROP COLUMN is_draft;

-----------------

DROP TABLE IF EXISTS rules;
DROP TABLE IF EXISTS campaigns_threads;
DROP TABLE IF EXISTS campaigns;
DROP TABLE IF EXISTS comments;
DROP TABLE IF EXISTS threads;

COMMIT;
