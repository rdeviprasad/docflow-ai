CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE intake_sessions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    organization_id UUID NOT NULL,
    client_name VARCHAR(255) NOT NULL,
    client_email VARCHAR(255) NOT NULL,
    magic_token VARCHAR(64) UNIQUE NOT NULL,
    status VARCHAR(50) DEFAULT 'PENDING',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE checklist_items (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id UUID REFERENCES intake_sessions(id) ON DELETE CASCADE,
    document_type VARCHAR(100) NOT NULL,
    description TEXT,
    is_required BOOLEAN DEFAULT TRUE,
    status VARCHAR(50) DEFAULT 'PENDING',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE document_submissions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    checklist_item_id UUID REFERENCES checklist_items(id) ON DELETE CASCADE,
    gcs_storage_path TEXT NOT NULL,
    original_filename VARCHAR(255) NOT NULL,
    file_mime_type VARCHAR(100) NOT NULL,
    file_size_bytes BIGINT NOT NULL,
    
    -- Model Evaluation Fields
    detected_document_type VARCHAR(100),
    is_valid BOOLEAN,
    confidence_score NUMERIC(4, 3),
    issues_detected JSONB DEFAULT '[]'::jsonb,
    extracted_metadata JSONB DEFAULT '{}'::jsonb,
    remediation_message TEXT,
    
    evaluated_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_sessions_token ON intake_sessions(magic_token);
CREATE INDEX idx_submissions_checklist ON document_submissions(checklist_item_id);