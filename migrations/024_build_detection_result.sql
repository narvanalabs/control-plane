-- Migration: 024_build_detection_result.sql
-- Add detection result fields to builds table for storing pre-build detection results
-- **Validates: Requirements 2.1**

-- Add detection_result JSONB column to store DetectionResult struct
ALTER TABLE builds
ADD COLUMN IF NOT EXISTS detection_result JSONB;

-- Add detected_at timestamp column to record when detection was performed
ALTER TABLE builds
ADD COLUMN IF NOT EXISTS detected_at TIMESTAMPTZ;

-- Add comments documenting the new columns
COMMENT ON COLUMN builds.detection_result IS 'JSONB containing DetectionResult from pre-build phase (strategy, framework, CGO detection, entry points, etc.)';
COMMENT ON COLUMN builds.detected_at IS 'Timestamp when detection was performed during pre-build phase';
