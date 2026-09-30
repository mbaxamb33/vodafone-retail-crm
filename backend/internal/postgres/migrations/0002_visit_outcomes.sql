-- Visit outcomes: request resolution, the next step agreed with the customer, contact consent
-- and whether a date was agreed. Earlier visits keep empty values.
ALTER TABLE visits
    ADD COLUMN next_action text NOT NULL DEFAULT '',
    ADD COLUMN next_action_label text NOT NULL DEFAULT '',
    ADD COLUMN next_action_details text NOT NULL DEFAULT '',
    ADD COLUMN contact_consent boolean NOT NULL DEFAULT false,
    ADD COLUMN agreed_due date,
    ADD COLUMN resolution_type text CHECK (resolution_type IN ('invoice', 'other')),
    ADD COLUMN resolution_holder text CHECK (resolution_holder IN ('holder', 'other')),
    ADD COLUMN resolution_status text CHECK (resolution_status IN ('resolved', 'unresolved', 'pending')),
    ADD CONSTRAINT visits_resolution_shape CHECK (
        (resolution_type IS NULL AND resolution_holder IS NULL AND resolution_status IS NULL)
        OR (resolution_type = 'invoice' AND resolution_holder IS NOT NULL AND resolution_status IS NULL)
        OR (resolution_type = 'other' AND resolution_status IS NOT NULL AND resolution_holder IS NULL)
    );

-- agreed: a date agreed with the customer; reminder: an internal check the employee set for
-- themselves; task: scheduled directly.
ALTER TABLE follow_ups ADD COLUMN kind text NOT NULL DEFAULT 'task' CHECK (kind IN ('agreed', 'reminder', 'task'));
UPDATE follow_ups SET kind = 'agreed' WHERE source_visit_id IS NOT NULL;

-- The next step agreed when the opportunity was identified, even without a date.
ALTER TABLE opportunities ADD COLUMN next_step text NOT NULL DEFAULT '';

-- Experience follow-ups start from the day this migration runs, not from all earlier history.
ALTER TABLE stores ADD COLUMN experience_since date NOT NULL DEFAULT current_date;

CREATE TABLE experience_checks (
    store_id    uuid NOT NULL REFERENCES stores (id),
    employee_id uuid NOT NULL REFERENCES users (id),
    customer_id uuid NOT NULL REFERENCES customers (id),
    day         date NOT NULL,
    status      text NOT NULL CHECK (status IN ('open', 'unreachable', 'done')),
    updated_at  timestamptz NOT NULL,
    PRIMARY KEY (employee_id, customer_id, day)
);

DELETE FROM catalog_items WHERE store_id IS NULL AND kind = 'next_action';
INSERT INTO catalog_items (store_id, kind, code, label, position) VALUES
    (NULL, 'next_action', 'none', 'Nimic', 10),
    (NULL, 'next_action', 'thinking', 'Se gândește și revine clientul', 20),
    (NULL, 'next_action', 'consulting', 'Se consultă cu altcineva', 30),
    (NULL, 'next_action', 'comparing', 'Caută alte oferte', 40),
    (NULL, 'next_action', 'not_interested', 'Nu este interesat', 50),
    (NULL, 'next_action', 'keep_in_touch', 'Rămânem în contact', 60),
    (NULL, 'next_action', 'other', 'Altceva', 70);
