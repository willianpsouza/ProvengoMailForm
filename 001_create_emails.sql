-- ============================================================================
-- Email Service - Database Schema (Gmail SMTP)
-- Executar no banco: sender_mail (usuario: sender_mail)
-- Host: 198.18.12.76
-- ============================================================================

CREATE TABLE IF NOT EXISTS emails (
    id          BIGSERIAL PRIMARY KEY,
    origem      VARCHAR(100)  NOT NULL,
    campanha    VARCHAR(50)   NOT NULL,
    email_src   VARCHAR(255)  NOT NULL,
    name        VARCHAR(255)  NOT NULL,
    assunto     VARCHAR(500)  NOT NULL,
    corpo       TEXT          NOT NULL,
    status      VARCHAR(20)   NOT NULL DEFAULT 'pending',  -- pending, sent, error
    smtp_error  TEXT          DEFAULT '',
    created_at  TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

-- Índices
CREATE INDEX IF NOT EXISTS idx_emails_origem     ON emails (origem);
CREATE INDEX IF NOT EXISTS idx_emails_campanha   ON emails (campanha);
CREATE INDEX IF NOT EXISTS idx_emails_status     ON emails (status);
CREATE INDEX IF NOT EXISTS idx_emails_created_at ON emails (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_emails_email_src  ON emails (email_src);

-- Trigger updated_at
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_emails_updated_at ON emails;
CREATE TRIGGER trg_emails_updated_at
    BEFORE UPDATE ON emails
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at();

COMMENT ON TABLE emails IS 'Registro de emails enviados via Gmail SMTP';
COMMENT ON COLUMN emails.origem IS 'Site de origem: provengo.io, voipe.io, etc';
COMMENT ON COLUMN emails.campanha IS 'Tipo: vendas, duvidas, contato';
COMMENT ON COLUMN emails.status IS 'Status do envio: pending, sent, error';
