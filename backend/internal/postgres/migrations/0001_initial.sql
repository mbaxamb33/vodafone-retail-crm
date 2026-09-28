CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;

CREATE TABLE stores (
    id         uuid PRIMARY KEY,
    name       text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    timezone   text NOT NULL DEFAULT 'Europe/Bucharest',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id            uuid PRIMARY KEY,
    store_id      uuid NOT NULL REFERENCES stores (id),
    email         text NOT NULL CHECK (email = lower(email)),
    name          text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    role          text NOT NULL CHECK (role IN ('employee', 'manager')),
    password_hash text NOT NULL,
    active        boolean NOT NULL DEFAULT true,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_email_key ON users (email);
CREATE INDEX users_store_idx ON users (store_id);

-- Only a SHA-256 digest of the session token is stored.
CREATE TABLE sessions (
    token_hash   bytea PRIMARY KEY,
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at   timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sessions_user_idx ON sessions (user_id);
CREATE INDEX sessions_expiry_idx ON sessions (expires_at);

CREATE TABLE login_attempts (
    id        bigserial PRIMARY KEY,
    email     text NOT NULL,
    client_ip text NOT NULL,
    success   boolean NOT NULL,
    at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX login_attempts_email_idx ON login_attempts (email, at);
CREATE INDEX login_attempts_ip_idx ON login_attempts (client_ip, at);

CREATE TABLE customers (
    id                  uuid PRIMARY KEY,
    store_id            uuid NOT NULL REFERENCES stores (id),
    name                text NOT NULL,
    search_name         text NOT NULL,
    phone               text NOT NULL,
    status              text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived', 'anonymized')),
    ownership           text NOT NULL CHECK (ownership IN ('owned', 'pool', 'unassigned')),
    owner_id            uuid REFERENCES users (id),
    tags                text[] NOT NULL DEFAULT '{}',
    created_by          uuid NOT NULL REFERENCES users (id),
    created_at          timestamptz NOT NULL,
    updated_at          timestamptz NOT NULL,
    last_interaction_at timestamptz,
    CHECK ((ownership = 'owned') = (owner_id IS NOT NULL))
);
CREATE INDEX customers_store_phone_idx ON customers (store_id, phone);
CREATE INDEX customers_store_owner_idx ON customers (store_id, owner_id);
CREATE INDEX customers_store_recent_idx ON customers (store_id, (coalesce(last_interaction_at, created_at)) DESC);
CREATE INDEX customers_search_name_trgm ON customers USING gin (search_name gin_trgm_ops);

CREATE TABLE catalog_items (
    id       bigserial PRIMARY KEY,
    store_id uuid REFERENCES stores (id),
    kind     text NOT NULL CHECK (kind IN ('visit_reason', 'next_action', 'product_category')),
    code     text NOT NULL,
    label    text NOT NULL,
    position integer NOT NULL DEFAULT 0,
    active   boolean NOT NULL DEFAULT true
);
-- A store-specific entry overrides the global (NULL store) entry with the same code.
CREATE UNIQUE INDEX catalog_items_key ON catalog_items (coalesce(store_id, '00000000-0000-0000-0000-000000000000'::uuid), kind, code);

CREATE TABLE visits (
    id            uuid PRIMARY KEY,
    store_id      uuid NOT NULL REFERENCES stores (id),
    customer_id   uuid NOT NULL REFERENCES customers (id),
    employee_id   uuid NOT NULL REFERENCES users (id),
    occurred_at   timestamptz NOT NULL,
    reason_code   text NOT NULL DEFAULT '',
    reason        text NOT NULL DEFAULT '',
    steps         smallint[] NOT NULL CHECK (cardinality(steps) > 0),
    furthest_step smallint NOT NULL CHECK (furthest_step BETWEEN 0 AND 7),
    notes         text NOT NULL DEFAULT '',
    notes_edited_at timestamptz
);
CREATE INDEX visits_customer_idx ON visits (customer_id, occurred_at DESC);
CREATE INDEX visits_store_time_idx ON visits (store_id, occurred_at);

-- Earlier versions of edited visit notes; the visit row always holds the current text.
CREATE TABLE visit_note_revisions (
    id         uuid PRIMARY KEY,
    visit_id   uuid NOT NULL REFERENCES visits (id),
    notes      text NOT NULL,
    replaced_by uuid NOT NULL REFERENCES users (id),
    replaced_at timestamptz NOT NULL
);
CREATE INDEX visit_note_revisions_visit_idx ON visit_note_revisions (visit_id);

CREATE TABLE opportunities (
    id              uuid PRIMARY KEY,
    store_id        uuid NOT NULL REFERENCES stores (id),
    customer_id     uuid NOT NULL REFERENCES customers (id),
    employee_id     uuid NOT NULL REFERENCES users (id),
    source_visit_id uuid REFERENCES visits (id),
    product         text NOT NULL CHECK (length(product) BETWEEN 1 AND 120),
    category        text NOT NULL DEFAULT '',
    stage           text NOT NULL CHECK (stage IN ('identified', 'qualified', 'verification', 'presentation', 'offer', 'waiting', 'won', 'lost', 'paused')),
    estimated_value numeric(12, 2) CHECK (estimated_value >= 0),
    notes           text NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL,
    updated_at      timestamptz NOT NULL,
    stage_changed_at timestamptz NOT NULL,
    closed_at       timestamptz
);
CREATE INDEX opportunities_customer_idx ON opportunities (customer_id);
CREATE INDEX opportunities_store_stage_idx ON opportunities (store_id, stage);
CREATE INDEX opportunities_employee_idx ON opportunities (employee_id, stage);

CREATE TABLE opportunity_stage_events (
    id             uuid PRIMARY KEY,
    store_id       uuid NOT NULL REFERENCES stores (id),
    opportunity_id uuid NOT NULL REFERENCES opportunities (id),
    customer_id    uuid NOT NULL REFERENCES customers (id),
    actor_id       uuid NOT NULL REFERENCES users (id),
    from_stage     text,
    to_stage       text NOT NULL,
    at             timestamptz NOT NULL
);
CREATE INDEX opportunity_stage_events_store_time_idx ON opportunity_stage_events (store_id, at);

CREATE TABLE follow_ups (
    id             uuid PRIMARY KEY,
    store_id       uuid NOT NULL REFERENCES stores (id),
    customer_id    uuid NOT NULL REFERENCES customers (id),
    employee_id    uuid NOT NULL REFERENCES users (id),
    opportunity_id uuid REFERENCES opportunities (id),
    source_visit_id uuid REFERENCES visits (id),
    type           text NOT NULL CHECK (length(type) BETWEEN 1 AND 100),
    due            date NOT NULL,
    status         text NOT NULL CHECK (status IN ('open', 'waiting', 'unreachable', 'done')),
    notes          text NOT NULL DEFAULT '',
    created_by     uuid NOT NULL REFERENCES users (id),
    created_at     timestamptz NOT NULL,
    updated_at     timestamptz NOT NULL,
    completed_at   timestamptz,
    CHECK ((status = 'done') = (completed_at IS NOT NULL))
);
CREATE INDEX follow_ups_employee_idx ON follow_ups (employee_id, status, due);
CREATE INDEX follow_ups_customer_idx ON follow_ups (customer_id);
CREATE INDEX follow_ups_store_idx ON follow_ups (store_id, status, due);

-- Append-only record of business actions. data holds structured details for reporting and display.
CREATE TABLE audit_events (
    id          uuid PRIMARY KEY,
    store_id    uuid NOT NULL REFERENCES stores (id),
    customer_id uuid REFERENCES customers (id),
    actor_id    uuid NOT NULL REFERENCES users (id),
    action      text NOT NULL,
    entity_type text NOT NULL,
    entity_id   uuid NOT NULL,
    detail      text NOT NULL DEFAULT '',
    data        jsonb NOT NULL DEFAULT '{}',
    at          timestamptz NOT NULL
);
CREATE INDEX audit_events_customer_idx ON audit_events (customer_id, at DESC);
CREATE INDEX audit_events_store_idx ON audit_events (store_id, at DESC);

CREATE TABLE notifications (
    id          uuid PRIMARY KEY,
    store_id    uuid NOT NULL REFERENCES stores (id),
    user_id     uuid NOT NULL REFERENCES users (id),
    kind        text NOT NULL,
    customer_id uuid REFERENCES customers (id),
    entity_id   uuid,
    message     text NOT NULL,
    created_at  timestamptz NOT NULL,
    read_at     timestamptz
);
CREATE INDEX notifications_user_idx ON notifications (user_id, created_at DESC);

INSERT INTO catalog_items (store_id, kind, code, label, position) VALUES
    (NULL, 'visit_reason', 'support', 'Service / suport', 10),
    (NULL, 'visit_reason', 'billing', 'Factură', 20),
    (NULL, 'visit_reason', 'new_subscription', 'Abonament nou', 30),
    (NULL, 'visit_reason', 'renewal', 'Reînnoire abonament', 40),
    (NULL, 'visit_reason', 'device', 'Telefon mobil', 50),
    (NULL, 'visit_reason', 'mobile_plan', 'Plan mobil', 60),
    (NULL, 'visit_reason', 'internet', 'Internet', 70),
    (NULL, 'visit_reason', 'tv', 'TV', 80),
    (NULL, 'visit_reason', 'accessories', 'Accesorii', 90),
    (NULL, 'visit_reason', 'contract_question', 'Întrebare despre contract', 100),
    (NULL, 'visit_reason', 'technical_issue', 'Problemă tehnică', 110),
    (NULL, 'visit_reason', 'other', 'Altele', 120),
    (NULL, 'next_action', 'call', 'Sună clientul', 10),
    (NULL, 'next_action', 'customer_returns', 'Clientul revine', 20),
    (NULL, 'next_action', 'check_eligibility', 'Verifică eligibilitatea', 30),
    (NULL, 'next_action', 'prepare_offer', 'Pregătește oferta', 40),
    (NULL, 'next_action', 'discuss_renewal', 'Discută reînnoirea', 50),
    (NULL, 'next_action', 'after_contract_expiry', 'Revino după expirarea contractului', 60),
    (NULL, 'next_action', 'waiting_customer', 'Așteptăm clientul', 70),
    (NULL, 'next_action', 'other', 'Altele', 80),
    (NULL, 'product_category', 'mobile', 'Abonament mobil', 10),
    (NULL, 'product_category', 'device', 'Telefon / dispozitiv', 20),
    (NULL, 'product_category', 'home_internet', 'Internet acasă', 30),
    (NULL, 'product_category', 'tv', 'TV', 40),
    (NULL, 'product_category', 'renewal', 'Reînnoire', 50),
    (NULL, 'product_category', 'accessories', 'Accesorii', 60),
    (NULL, 'product_category', 'other', 'Altele', 70);
