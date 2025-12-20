-- Narvana Control Plane Build Queue Schema
-- Migration: 002_build_queue.sql

-- Build queue table: stores pending build jobs for worker processing
CREATE TABLE build_queue (
    id UUID PRIMARY KEY,
    job_data JSONB NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing')),
    retry_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ
);

-- Indexes for build_queue
CREATE INDEX idx_build_queue_status ON build_queue(status);
CREATE INDEX idx_build_queue_created_at ON build_queue(created_at ASC);
CREATE INDEX idx_build_queue_pending ON build_queue(created_at ASC) WHERE status = 'pending';
