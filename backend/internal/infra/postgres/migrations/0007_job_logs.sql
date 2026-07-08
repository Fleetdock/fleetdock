-- =============================================================================
-- Migration 0007 — per-operation execution logs. Both executors (the
-- control-plane worker and remote agents) append step-level and stderr lines
-- here during a job's run, so the operation detail page can show a live,
-- tailing log stream instead of just the single final error string.
-- (No BEGIN/COMMIT: the migration runner wraps each file in a transaction.)
-- =============================================================================

CREATE TABLE job_logs (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  job_id     uuid NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  seq        integer NOT NULL,                 -- monotonic per job (assigned by the sink)
  level      text NOT NULL DEFAULT 'info'
               CHECK (level IN ('info','warn','error','stderr')),
  message    text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

-- Tail queries filter by (job_id, seq > afterSeq) and order by seq.
CREATE INDEX ix_job_logs_job ON job_logs (job_id, seq);
