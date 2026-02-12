-- Entitlements-as-a-Service Database Schema
-- PostgreSQL DDL for all 5 tables

CREATE TABLE IF NOT EXISTS subscribers (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    imsi VARCHAR(15) UNIQUE NOT NULL,
    msisdn VARCHAR(15),
    iccid VARCHAR(20),
    eid VARCHAR(32),
    ki_encrypted BYTEA NOT NULL,
    op_encrypted BYTEA NOT NULL,
    ki_dek_wrapped BYTEA NOT NULL,
    sqn BIGINT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS devices (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    subscriber_id UUID REFERENCES subscribers(id),
    imei VARCHAR(15) NOT NULL,
    vendor VARCHAR(128),
    model VARCHAR(128),
    sw_version VARCHAR(128),
    device_type VARCHAR(64),
    eid VARCHAR(32),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS entitlements (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    subscriber_id UUID NOT NULL REFERENCES subscribers(id),
    app_id VARCHAR(10) NOT NULL,
    status INTEGER NOT NULL DEFAULT 0,
    prov_status INTEGER DEFAULT 0,
    tc_status INTEGER DEFAULT 0,
    config_data JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(subscriber_id, app_id)
);

CREATE TABLE IF NOT EXISTS tokens (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    subscriber_id UUID NOT NULL REFERENCES subscribers(id),
    token_value VARCHAR(512) UNIQUE NOT NULL,
    token_type VARCHAR(32) NOT NULL,
    issued_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    scope VARCHAR(256),
    operation_targets TEXT[],
    consumed BOOLEAN DEFAULT FALSE,
    created_by_ip INET
);

CREATE TABLE IF NOT EXISTS audit_log (
    id BIGSERIAL PRIMARY KEY,
    timestamp TIMESTAMPTZ DEFAULT NOW(),
    subscriber_id UUID,
    app_id VARCHAR(10),
    operation VARCHAR(64),
    request_summary JSONB,
    response_code INTEGER,
    client_ip INET,
    user_agent TEXT
);
