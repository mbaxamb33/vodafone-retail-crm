package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"vodafone/store/internal/crm"
)

// args builds positional query parameters.
type args []any

func (a *args) add(v any) string {
	*a = append(*a, v)
	return fmt.Sprintf("$%d", len(*a))
}

func likePattern(s string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s) + "%"
}

func rangeCond(a *args, column string, r crm.TimeRange) string {
	if r.All() {
		return "true"
	}
	return fmt.Sprintf("%s >= %s AND %s < %s", column, a.add(r.From), column, a.add(r.To))
}

// ---- Stores, users, catalog ----

func (q *queries) StoreByID(ctx context.Context, storeID string) (crm.RetailStore, error) {
	var s crm.RetailStore
	err := q.q.QueryRow(ctx, `SELECT id::text, name, timezone, experience_since::text FROM stores WHERE id = $1`, storeID).Scan(&s.ID, &s.Name, &s.Timezone, &s.ExperienceSince)
	return s, mapErr(err)
}

func (q *queries) StoreUsers(ctx context.Context, storeID string) ([]crm.User, error) {
	rows, err := q.q.Query(ctx, `SELECT id::text, store_id::text, email, name, role, active FROM users WHERE store_id = $1 ORDER BY name`, storeID)
	if err != nil {
		return nil, mapErr(err)
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (crm.User, error) {
		var u crm.User
		return u, r.Scan(&u.ID, &u.StoreID, &u.Email, &u.Name, &u.Role, &u.Active)
	})
}

func (q *queries) Catalog(ctx context.Context, storeID string) ([]crm.CatalogItem, error) {
	rows, err := q.q.Query(ctx, `
		SELECT kind, code, label FROM (
			SELECT DISTINCT ON (kind, code) kind, code, label, position, active
			FROM catalog_items WHERE store_id IS NULL OR store_id = $1
			ORDER BY kind, code, store_id NULLS LAST
		) items WHERE active ORDER BY kind, position, code`, storeID)
	if err != nil {
		return nil, mapErr(err)
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (crm.CatalogItem, error) {
		var c crm.CatalogItem
		return c, r.Scan(&c.Kind, &c.Code, &c.Label)
	})
}

// ---- Customers ----

const customerCols = `c.id::text, c.store_id::text, c.name, c.phone, c.status, c.ownership, coalesce(c.owner_id::text, ''), c.tags,
	c.created_by::text, c.created_at, c.updated_at, c.last_interaction_at,
	coalesce((SELECT min(f.due)::text FROM follow_ups f WHERE f.customer_id = c.id AND f.status <> 'done'), '')`

func scanCustomer(r pgx.Row) (crm.Customer, error) {
	var c crm.Customer
	err := r.Scan(&c.ID, &c.StoreID, &c.Name, &c.Phone, &c.Status, &c.Ownership, &c.OwnerID, &c.Tags, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt, &c.LastInteractionAt, &c.NextFollowUpDue)
	if c.Tags == nil {
		c.Tags = []string{}
	}
	return c, mapErr(err)
}

func collectCustomers(rows pgx.Rows, err error) ([]crm.Customer, error) {
	if err != nil {
		return nil, mapErr(err)
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (crm.Customer, error) { return scanCustomer(r) })
	if out == nil {
		out = []crm.Customer{}
	}
	return out, mapErr(err)
}

func (q *queries) Customer(ctx context.Context, storeID, id string) (crm.Customer, error) {
	return scanCustomer(q.q.QueryRow(ctx, `SELECT `+customerCols+` FROM customers c WHERE c.store_id = $1 AND c.id = $2`, storeID, id))
}

func (q *queries) LockCustomer(ctx context.Context, storeID, id string) (crm.Customer, error) {
	return scanCustomer(q.q.QueryRow(ctx, `SELECT `+customerCols+` FROM customers c WHERE c.store_id = $1 AND c.id = $2 FOR UPDATE`, storeID, id))
}

func (q *queries) CustomersByID(ctx context.Context, storeID string, ids []string) ([]crm.Customer, error) {
	return collectCustomers(q.q.Query(ctx, `SELECT `+customerCols+` FROM customers c WHERE c.store_id = $1 AND c.id = ANY($2::uuid[])`, storeID, ids))
}

func (q *queries) Customers(ctx context.Context, storeID string, f crm.CustomerFilter) ([]crm.Customer, int, error) {
	a := args{storeID}
	where := []string{"c.store_id = $1"}
	if f.Status == "" {
		where = append(where, "c.status <> 'anonymized'")
	} else {
		where = append(where, "c.status = "+a.add(f.Status))
	}
	if f.Query != "" || f.Phone != "" {
		conds := []string{}
		if f.Query != "" {
			conds = append(conds, `c.search_name LIKE `+a.add(likePattern(f.Query)))
		}
		if f.Phone != "" {
			conds = append(conds, `replace(c.phone, '+', '') LIKE `+a.add(likePattern(f.Phone)))
		}
		where = append(where, "("+strings.Join(conds, " OR ")+")")
	}
	if f.ExactPhone != "" {
		where = append(where, "c.phone = "+a.add(f.ExactPhone))
	}
	if f.OwnerID != "" {
		where = append(where, "c.owner_id = "+a.add(f.OwnerID))
	}
	if f.Ownership != "" {
		where = append(where, "c.ownership = "+a.add(f.Ownership))
	}
	cond := strings.Join(where, " AND ")
	var total int
	if err := q.q.QueryRow(ctx, `SELECT count(*) FROM customers c WHERE `+cond, a...).Scan(&total); err != nil {
		return nil, 0, mapErr(err)
	}
	order := "coalesce(c.last_interaction_at, c.created_at) DESC, c.id"
	switch f.Sort {
	case "name":
		order = "coalesce(nullif(c.search_name, ''), c.phone), c.id"
	case "newest":
		order = "c.created_at DESC, c.id"
	case "followup":
		order = "(SELECT min(f.due) FROM follow_ups f WHERE f.customer_id = c.id AND f.status <> 'done') ASC NULLS LAST, c.id"
	}
	sql := `SELECT ` + customerCols + ` FROM customers c WHERE ` + cond + ` ORDER BY ` + order + ` LIMIT ` + a.add(f.Limit) + ` OFFSET ` + a.add(f.Offset)
	items, err := collectCustomers(q.q.Query(ctx, sql, a...))
	return items, total, err
}

func (q *queries) InsertCustomer(ctx context.Context, c crm.Customer) error {
	_, err := q.q.Exec(ctx, `INSERT INTO customers (id, store_id, name, search_name, phone, status, ownership, owner_id, tags, created_by, created_at, updated_at, last_interaction_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		c.ID, c.StoreID, c.Name, crm.FoldName(c.Name), c.Phone, c.Status, c.Ownership, nullable(c.OwnerID), c.Tags, c.CreatedBy, c.CreatedAt, c.UpdatedAt, c.LastInteractionAt)
	return mapErr(err)
}

func (q *queries) UpdateCustomer(ctx context.Context, c crm.Customer) error {
	_, err := q.q.Exec(ctx, `UPDATE customers SET name = $3, search_name = $4, phone = $5, status = $6, ownership = $7, owner_id = $8, tags = $9, updated_at = $10, last_interaction_at = $11
		WHERE store_id = $1 AND id = $2`,
		c.StoreID, c.ID, c.Name, crm.FoldName(c.Name), c.Phone, c.Status, c.Ownership, nullable(c.OwnerID), c.Tags, c.UpdatedAt, c.LastInteractionAt)
	return mapErr(err)
}

// AnonymizeCustomer removes identifying data and free text while keeping counts and stages.
func (q *queries) AnonymizeCustomer(ctx context.Context, storeID, id string, at time.Time) error {
	if _, err := q.q.Exec(ctx, `UPDATE customers SET name = 'Client anonimizat', search_name = 'client anonimizat', phone = '', tags = '{}', status = 'anonymized',
		ownership = 'unassigned', owner_id = NULL, updated_at = $3 WHERE store_id = $1 AND id = $2`, storeID, id, at); err != nil {
		return mapErr(err)
	}
	for _, s := range []string{
		`DELETE FROM visit_note_revisions WHERE visit_id IN (SELECT id FROM visits WHERE store_id = $1 AND customer_id = $2)`,
		`UPDATE visits SET notes = '', notes_edited_at = NULL WHERE store_id = $1 AND customer_id = $2`,
		`UPDATE follow_ups SET notes = '' WHERE store_id = $1 AND customer_id = $2`,
		`UPDATE opportunities SET notes = '' WHERE store_id = $1 AND customer_id = $2`,
	} {
		if _, err := q.q.Exec(ctx, s, storeID, id); err != nil {
			return mapErr(err)
		}
	}
	return nil
}

// ---- Visits ----

const visitCols = `v.id::text, v.store_id::text, v.customer_id::text, v.employee_id::text, v.occurred_at, v.reason_code, v.reason, v.steps, v.furthest_step, v.notes, v.notes_edited_at,
	v.next_action, v.next_action_label, v.next_action_details, v.contact_consent, coalesce(v.agreed_due::text, ''),
	coalesce(v.resolution_type, ''), coalesce(v.resolution_holder, ''), coalesce(v.resolution_status, '')`

func scanVisit(r pgx.Row) (crm.Visit, error) {
	var v crm.Visit
	var steps []int16
	var furthest int16
	var res crm.Resolution
	d := &v.Details
	err := r.Scan(&v.ID, &v.StoreID, &v.CustomerID, &v.EmployeeID, &v.At, &v.ReasonCode, &v.Reason, &steps, &furthest, &v.Notes, &v.NotesEditedAt,
		&d.NextAction, &d.NextActionLabel, &d.ActionDetails, &d.ContactConsent, &d.Due, &res.Type, &res.Holder, &res.Status)
	d.AgreedDate = d.Due != ""
	if res.Type != "" {
		d.Resolution = &res
	}
	v.Steps = make([]int, len(steps))
	for i, s := range steps {
		v.Steps[i] = int(s)
	}
	v.FurthestStep = int(furthest)
	return v, mapErr(err)
}

func (q *queries) Visit(ctx context.Context, storeID, id string) (crm.Visit, error) {
	return scanVisit(q.q.QueryRow(ctx, `SELECT `+visitCols+` FROM visits v WHERE v.store_id = $1 AND v.id = $2`, storeID, id))
}

func (q *queries) LockVisit(ctx context.Context, storeID, id string) (crm.Visit, error) {
	return scanVisit(q.q.QueryRow(ctx, `SELECT `+visitCols+` FROM visits v WHERE v.store_id = $1 AND v.id = $2 FOR UPDATE`, storeID, id))
}

func (q *queries) Visits(ctx context.Context, storeID string, f crm.VisitFilter) ([]crm.Visit, int, error) {
	a := args{storeID}
	where := []string{"v.store_id = $1", rangeCond(&a, "v.occurred_at", f.Range)}
	if f.CustomerID != "" {
		where = append(where, "v.customer_id = "+a.add(f.CustomerID))
	}
	if f.EmployeeID != "" {
		where = append(where, "v.employee_id = "+a.add(f.EmployeeID))
	}
	cond := strings.Join(where, " AND ")
	var total int
	if err := q.q.QueryRow(ctx, `SELECT count(*) FROM visits v WHERE `+cond, a...).Scan(&total); err != nil {
		return nil, 0, mapErr(err)
	}
	rows, err := q.q.Query(ctx, `SELECT `+visitCols+` FROM visits v WHERE `+cond+` ORDER BY v.occurred_at DESC, v.id LIMIT `+a.add(f.Limit)+` OFFSET `+a.add(f.Offset), a...)
	if err != nil {
		return nil, 0, mapErr(err)
	}
	items, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (crm.Visit, error) { return scanVisit(r) })
	if items == nil {
		items = []crm.Visit{}
	}
	return items, total, mapErr(err)
}

func (q *queries) InsertVisit(ctx context.Context, v crm.Visit) error {
	d := v.Details
	var resType, holder, status string
	if d.Resolution != nil {
		resType, holder, status = d.Resolution.Type, d.Resolution.Holder, d.Resolution.Status
	}
	_, err := q.q.Exec(ctx, `INSERT INTO visits (id, store_id, customer_id, employee_id, occurred_at, reason_code, reason, steps, furthest_step, notes,
			next_action, next_action_label, next_action_details, contact_consent, agreed_due, resolution_type, resolution_holder, resolution_status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15::date, $16, $17, $18)`,
		v.ID, v.StoreID, v.CustomerID, v.EmployeeID, v.At, v.ReasonCode, v.Reason, v.Steps, v.FurthestStep, v.Notes,
		d.NextAction, d.NextActionLabel, d.ActionDetails, d.ContactConsent, nullable(d.Due), nullable(resType), nullable(holder), nullable(status))
	return mapErr(err)
}

func (q *queries) UpdateVisitNotes(ctx context.Context, v crm.Visit, previous, editorID, revisionID string) error {
	if _, err := q.q.Exec(ctx, `INSERT INTO visit_note_revisions (id, visit_id, notes, replaced_by, replaced_at) VALUES ($1, $2, $3, $4, $5)`,
		revisionID, v.ID, previous, editorID, v.NotesEditedAt); err != nil {
		return mapErr(err)
	}
	_, err := q.q.Exec(ctx, `UPDATE visits SET notes = $3, notes_edited_at = $4 WHERE store_id = $1 AND id = $2`, v.StoreID, v.ID, v.Notes, v.NotesEditedAt)
	return mapErr(err)
}

// ---- Opportunities ----

const opportunityCols = `o.id::text, o.store_id::text, o.customer_id::text, o.employee_id::text, coalesce(o.source_visit_id::text, ''), o.product, o.category, o.stage, o.next_step,
	o.estimated_value::float8, o.notes, o.created_at, o.updated_at, o.stage_changed_at, o.closed_at`

func scanOpportunity(r pgx.Row) (crm.Opportunity, error) {
	var o crm.Opportunity
	err := r.Scan(&o.ID, &o.StoreID, &o.CustomerID, &o.EmployeeID, &o.SourceVisitID, &o.Product, &o.Category, &o.Stage, &o.NextStep, &o.EstimatedValue, &o.Notes, &o.CreatedAt, &o.UpdatedAt, &o.StageChangedAt, &o.ClosedAt)
	return o, mapErr(err)
}

func (q *queries) LockOpportunity(ctx context.Context, storeID, id string) (crm.Opportunity, error) {
	return scanOpportunity(q.q.QueryRow(ctx, `SELECT `+opportunityCols+` FROM opportunities o WHERE o.store_id = $1 AND o.id = $2 FOR UPDATE`, storeID, id))
}

func (q *queries) Opportunities(ctx context.Context, storeID string, f crm.OpportunityFilter) ([]crm.Opportunity, int, error) {
	a := args{storeID}
	where := []string{"o.store_id = $1"}
	if f.CustomerID != "" {
		where = append(where, "o.customer_id = "+a.add(f.CustomerID))
	}
	if f.EmployeeID != "" {
		where = append(where, "o.employee_id = "+a.add(f.EmployeeID))
	}
	if len(f.Stages) > 0 {
		where = append(where, "o.stage = ANY("+a.add(f.Stages)+")")
	}
	if f.ActiveOnly {
		where = append(where, "o.stage NOT IN ('won', 'lost')")
	} else if f.ClosedSince != nil {
		where = append(where, "(o.stage NOT IN ('won', 'lost') OR o.closed_at >= "+a.add(*f.ClosedSince)+")")
	}
	cond := strings.Join(where, " AND ")
	var total int
	if err := q.q.QueryRow(ctx, `SELECT count(*) FROM opportunities o WHERE `+cond, a...).Scan(&total); err != nil {
		return nil, 0, mapErr(err)
	}
	rows, err := q.q.Query(ctx, `SELECT `+opportunityCols+` FROM opportunities o WHERE `+cond+` ORDER BY o.created_at DESC, o.id LIMIT `+a.add(f.Limit)+` OFFSET `+a.add(f.Offset), a...)
	if err != nil {
		return nil, 0, mapErr(err)
	}
	items, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (crm.Opportunity, error) { return scanOpportunity(r) })
	if items == nil {
		items = []crm.Opportunity{}
	}
	return items, total, mapErr(err)
}

func (q *queries) InsertOpportunity(ctx context.Context, o crm.Opportunity) error {
	_, err := q.q.Exec(ctx, `INSERT INTO opportunities (id, store_id, customer_id, employee_id, source_visit_id, product, category, stage, estimated_value, notes, created_at, updated_at, stage_changed_at, closed_at, next_step)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
		o.ID, o.StoreID, o.CustomerID, o.EmployeeID, nullable(o.SourceVisitID), o.Product, o.Category, o.Stage, o.EstimatedValue, o.Notes, o.CreatedAt, o.UpdatedAt, o.StageChangedAt, o.ClosedAt, o.NextStep)
	return mapErr(err)
}

func (q *queries) UpdateOpportunity(ctx context.Context, o crm.Opportunity) error {
	_, err := q.q.Exec(ctx, `UPDATE opportunities SET product = $3, category = $4, stage = $5, estimated_value = $6, notes = $7, updated_at = $8, stage_changed_at = $9, closed_at = $10
		WHERE store_id = $1 AND id = $2`,
		o.StoreID, o.ID, o.Product, o.Category, o.Stage, o.EstimatedValue, o.Notes, o.UpdatedAt, o.StageChangedAt, o.ClosedAt)
	return mapErr(err)
}

func (q *queries) InsertStageEvent(ctx context.Context, e crm.StageEvent) error {
	_, err := q.q.Exec(ctx, `INSERT INTO opportunity_stage_events (id, store_id, opportunity_id, customer_id, actor_id, from_stage, to_stage, at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		e.ID, e.StoreID, e.OpportunityID, e.CustomerID, e.ActorID, nullable(e.FromStage), e.ToStage, e.At)
	return mapErr(err)
}

// ---- Follow-ups ----

const followUpCols = `f.id::text, f.store_id::text, f.customer_id::text, f.employee_id::text, coalesce(f.opportunity_id::text, ''), coalesce(f.source_visit_id::text, ''), f.kind,
	f.type, f.due::text, f.status, f.notes, f.created_by::text, f.created_at, f.updated_at, f.completed_at`

func scanFollowUp(r pgx.Row) (crm.FollowUp, error) {
	var f crm.FollowUp
	err := r.Scan(&f.ID, &f.StoreID, &f.CustomerID, &f.EmployeeID, &f.OpportunityID, &f.SourceVisitID, &f.Kind, &f.Type, &f.Due, &f.Status, &f.Notes, &f.CreatedBy, &f.CreatedAt, &f.UpdatedAt, &f.CompletedAt)
	return f, mapErr(err)
}

func (q *queries) LockFollowUp(ctx context.Context, storeID, id string) (crm.FollowUp, error) {
	return scanFollowUp(q.q.QueryRow(ctx, `SELECT `+followUpCols+` FROM follow_ups f WHERE f.store_id = $1 AND f.id = $2 FOR UPDATE`, storeID, id))
}

func (q *queries) FollowUps(ctx context.Context, storeID string, f crm.FollowUpFilter) ([]crm.FollowUp, int, error) {
	a := args{storeID}
	where := []string{"f.store_id = $1"}
	add := func(cond string, v any) { where = append(where, fmt.Sprintf(cond, a.add(v))) }
	if f.CustomerID != "" {
		add("f.customer_id = %s", f.CustomerID)
	}
	if f.EmployeeID != "" {
		add("f.employee_id = %s", f.EmployeeID)
	}
	if f.OpportunityID != "" {
		add("f.opportunity_id = %s", f.OpportunityID)
	}
	if len(f.Statuses) > 0 {
		add("f.status = ANY(%s)", f.Statuses)
	}
	if f.DueBefore != "" {
		add("f.due < %s::date", f.DueBefore)
	}
	if f.DueFrom != "" {
		add("f.due >= %s::date", f.DueFrom)
	}
	if f.DueTo != "" {
		add("f.due <= %s::date", f.DueTo)
	}
	if f.CompletedFrom != nil {
		add("f.completed_at >= %s", *f.CompletedFrom)
	}
	if f.OpenOrCompletedSince != nil {
		add("(f.status <> 'done' OR f.completed_at >= %s)", *f.OpenOrCompletedSince)
	}
	cond := strings.Join(where, " AND ")
	var total int
	if err := q.q.QueryRow(ctx, `SELECT count(*) FROM follow_ups f WHERE `+cond, a...).Scan(&total); err != nil {
		return nil, 0, mapErr(err)
	}
	rows, err := q.q.Query(ctx, `SELECT `+followUpCols+` FROM follow_ups f WHERE `+cond+` ORDER BY f.due, f.created_at, f.id LIMIT `+a.add(f.Limit)+` OFFSET `+a.add(f.Offset), a...)
	if err != nil {
		return nil, 0, mapErr(err)
	}
	items, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (crm.FollowUp, error) { return scanFollowUp(r) })
	if items == nil {
		items = []crm.FollowUp{}
	}
	return items, total, mapErr(err)
}

func (q *queries) InsertFollowUp(ctx context.Context, f crm.FollowUp) error {
	_, err := q.q.Exec(ctx, `INSERT INTO follow_ups (id, store_id, customer_id, employee_id, opportunity_id, source_visit_id, type, due, status, notes, created_by, created_at, updated_at, completed_at, kind)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::date, $9, $10, $11, $12, $13, $14, $15)`,
		f.ID, f.StoreID, f.CustomerID, f.EmployeeID, nullable(f.OpportunityID), nullable(f.SourceVisitID), f.Type, f.Due, f.Status, f.Notes, f.CreatedBy, f.CreatedAt, f.UpdatedAt, f.CompletedAt, f.Kind)
	return mapErr(err)
}

func (q *queries) UpdateFollowUp(ctx context.Context, f crm.FollowUp) error {
	_, err := q.q.Exec(ctx, `UPDATE follow_ups SET due = $3::date, status = $4, notes = $5, updated_at = $6, completed_at = $7 WHERE store_id = $1 AND id = $2`,
		f.StoreID, f.ID, f.Due, f.Status, f.Notes, f.UpdatedAt, f.CompletedAt)
	return mapErr(err)
}

// ---- Audit and notifications ----

func (q *queries) InsertAudit(ctx context.Context, e crm.Audit) error {
	_, err := q.q.Exec(ctx, `INSERT INTO audit_events (id, store_id, customer_id, actor_id, action, entity_type, entity_id, detail, data, at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		e.ID, e.StoreID, nullable(e.CustomerID), e.ActorID, e.Action, e.EntityType, e.EntityID, e.Detail, string(e.Data), e.At)
	return mapErr(err)
}

func (q *queries) Audit(ctx context.Context, storeID string, f crm.AuditFilter) ([]crm.Audit, error) {
	a := args{storeID}
	where := []string{"e.store_id = $1"}
	if f.CustomerID != "" {
		where = append(where, "e.customer_id = "+a.add(f.CustomerID))
	}
	if f.ActorID != "" {
		where = append(where, "e.actor_id = "+a.add(f.ActorID))
	}
	if len(f.Actions) > 0 {
		where = append(where, "e.action = ANY("+a.add(f.Actions)+")")
	}
	if f.Since != nil {
		where = append(where, "e.at >= "+a.add(*f.Since))
	}
	rows, err := q.q.Query(ctx, `SELECT e.id::text, e.store_id::text, coalesce(e.customer_id::text, ''), e.actor_id::text, e.action, e.entity_type, e.entity_id::text, e.detail, e.data, e.at
		FROM audit_events e WHERE `+strings.Join(where, " AND ")+` ORDER BY e.at DESC, e.id LIMIT `+a.add(f.Limit), a...)
	if err != nil {
		return nil, mapErr(err)
	}
	items, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (crm.Audit, error) {
		var e crm.Audit
		var data []byte
		err := r.Scan(&e.ID, &e.StoreID, &e.CustomerID, &e.ActorID, &e.Action, &e.EntityType, &e.EntityID, &e.Detail, &data, &e.At)
		e.Data = data
		return e, err
	})
	if items == nil {
		items = []crm.Audit{}
	}
	return items, mapErr(err)
}

func (q *queries) InsertNotification(ctx context.Context, n crm.Notification) error {
	_, err := q.q.Exec(ctx, `INSERT INTO notifications (id, store_id, user_id, kind, customer_id, entity_id, message, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		n.ID, n.StoreID, n.UserID, n.Kind, nullable(n.CustomerID), nullable(n.EntityID), n.Message, n.CreatedAt)
	return mapErr(err)
}

func (q *queries) Notifications(ctx context.Context, storeID, userID string, unreadOnly bool, limit int) ([]crm.Notification, int, error) {
	cond := "n.store_id = $1 AND n.user_id = $2"
	if unreadOnly {
		cond += " AND n.read_at IS NULL"
	}
	var total int
	if err := q.q.QueryRow(ctx, `SELECT count(*) FROM notifications n WHERE `+cond, storeID, userID).Scan(&total); err != nil {
		return nil, 0, mapErr(err)
	}
	rows, err := q.q.Query(ctx, `SELECT n.id::text, n.store_id::text, n.user_id::text, n.kind, coalesce(n.customer_id::text, ''), coalesce(c.name, ''), coalesce(n.entity_id::text, ''), n.message, n.created_at, n.read_at
		FROM notifications n LEFT JOIN customers c ON c.id = n.customer_id WHERE `+cond+` ORDER BY n.created_at DESC, n.id LIMIT $3`, storeID, userID, limit)
	if err != nil {
		return nil, 0, mapErr(err)
	}
	items, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (crm.Notification, error) {
		var n crm.Notification
		return n, r.Scan(&n.ID, &n.StoreID, &n.UserID, &n.Kind, &n.CustomerID, &n.CustomerName, &n.EntityID, &n.Message, &n.CreatedAt, &n.ReadAt)
	})
	if items == nil {
		items = []crm.Notification{}
	}
	return items, total, mapErr(err)
}

func (q *queries) MarkNotificationsRead(ctx context.Context, storeID, userID string, ids []string, at time.Time) error {
	sql := `UPDATE notifications SET read_at = $3 WHERE store_id = $1 AND user_id = $2 AND read_at IS NULL`
	a := args{storeID, userID, at}
	if len(ids) > 0 {
		sql += " AND id = ANY(" + a.add(ids) + "::uuid[])"
	}
	_, err := q.q.Exec(ctx, sql, a...)
	return mapErr(err)
}

func (q *queries) NotifiedSince(ctx context.Context, storeID, userID, kind string, since time.Time) (bool, error) {
	var exists bool
	err := q.q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM notifications WHERE store_id = $1 AND user_id = $2 AND kind = $3 AND created_at >= $4)`, storeID, userID, kind, since).Scan(&exists)
	return exists, mapErr(err)
}

// ---- Experience follow-ups ----

func (q *queries) ExperienceTasks(ctx context.Context, storeID, employeeID, timezone, from, to string) ([]crm.ExperienceTask, error) {
	rows, err := q.q.Query(ctx, `
		SELECT d.customer_id::text, d.day::text, coalesce(x.status, 'open')
		FROM (
			SELECT DISTINCT customer_id, (occurred_at AT TIME ZONE $3)::date AS day
			FROM visits WHERE store_id = $1 AND employee_id = $2
		) d
		JOIN customers c ON c.id = d.customer_id AND c.status <> 'anonymized'
		LEFT JOIN experience_checks x ON x.employee_id = $2 AND x.customer_id = d.customer_id AND x.day = d.day
		WHERE d.day >= $4::date AND d.day < $5::date
		ORDER BY d.day, c.created_at, d.customer_id`, storeID, employeeID, timezone, from, to)
	if err != nil {
		return nil, mapErr(err)
	}
	items, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (crm.ExperienceTask, error) {
		t := crm.ExperienceTask{EmployeeID: employeeID}
		err := r.Scan(&t.CustomerID, &t.Day, &t.Status)
		t.ID = t.CustomerID + ":" + t.Day
		return t, err
	})
	if items == nil {
		items = []crm.ExperienceTask{}
	}
	return items, mapErr(err)
}

func (q *queries) SetExperienceStatus(ctx context.Context, storeID string, t crm.ExperienceTask, at time.Time) error {
	_, err := q.q.Exec(ctx, `INSERT INTO experience_checks (store_id, employee_id, customer_id, day, status, updated_at) VALUES ($1, $2, $3, $4::date, $5, $6)
		ON CONFLICT (employee_id, customer_id, day) DO UPDATE SET status = EXCLUDED.status, updated_at = EXCLUDED.updated_at`,
		storeID, t.EmployeeID, t.CustomerID, t.Day, t.Status, at)
	return mapErr(err)
}

// ---- Reporting ----

func (q *queries) ReportCounts(ctx context.Context, storeID string, r crm.TimeRange, today string) (crm.ReportCounts, error) {
	a := args{storeID, today}
	visitRange := rangeCond(&a, "occurred_at", r)
	createdRange := rangeCond(&a, "created_at", r)
	eventRange := rangeCond(&a, "at", r)
	sql := `
		WITH v AS (SELECT customer_id, steps, furthest_step FROM visits WHERE store_id = $1 AND ` + visitRange + `)
		SELECT
			(SELECT count(*) FROM v),
			(SELECT count(DISTINCT customer_id) FROM v),
			(SELECT count(*) FROM customers WHERE store_id = $1 AND ` + createdRange + `),
			(SELECT count(*) FROM opportunities WHERE store_id = $1 AND ` + createdRange + `),
			(SELECT count(*) FILTER (WHERE to_stage = 'offer') FROM opportunity_stage_events WHERE store_id = $1 AND ` + eventRange + `),
			(SELECT count(*) FILTER (WHERE to_stage = 'won') FROM opportunity_stage_events WHERE store_id = $1 AND ` + eventRange + `),
			(SELECT count(*) FILTER (WHERE to_stage = 'lost') FROM opportunity_stage_events WHERE store_id = $1 AND ` + eventRange + `),
			(SELECT array_agg((SELECT count(*) FROM v WHERE furthest_step >= s) ORDER BY s) FROM generate_series(0, 7) s),
			(SELECT array_agg((SELECT count(*) FROM v WHERE s = ANY(steps)) ORDER BY s) FROM generate_series(0, 7) s),
			(SELECT count(*) FROM customers WHERE store_id = $1 AND ownership = 'pool' AND status = 'active'),
			(SELECT count(*) FROM opportunities WHERE store_id = $1 AND stage NOT IN ('won', 'lost')),
			(SELECT count(*) FROM follow_ups WHERE store_id = $1 AND status <> 'done' AND due = $2::date),
			(SELECT count(*) FROM follow_ups WHERE store_id = $1 AND status <> 'done' AND due < $2::date)`
	var c crm.ReportCounts
	var furthest, incidence []int64
	err := q.q.QueryRow(ctx, sql, a...).Scan(&c.Visits, &c.CustomersHandled, &c.NewCustomers, &c.OpportunitiesCreated, &c.Offers, &c.Won, &c.Lost,
		&furthest, &incidence, &c.PoolCustomers, &c.ActiveOpportunities, &c.FollowUpsDueToday, &c.OverdueFollowUps)
	for i := 0; i < 8 && i < len(furthest); i++ {
		c.FurthestAtLeast[i] = int(furthest[i])
		c.StepIncidence[i] = int(incidence[i])
	}
	return c, mapErr(err)
}

func (q *queries) EmployeeCounts(ctx context.Context, storeID string, r crm.TimeRange, today string) ([]crm.EmployeeCounts, error) {
	a := args{storeID, today}
	visitRange := rangeCond(&a, "v.occurred_at", r)
	customerRange := rangeCond(&a, "c.created_at", r)
	oppRange := rangeCond(&a, "o.created_at", r)
	eventRange := rangeCond(&a, "e.at", r)
	rows, err := q.q.Query(ctx, `
		SELECT u.id::text,
			(SELECT count(*) FROM visits v WHERE v.store_id = $1 AND v.employee_id = u.id AND `+visitRange+`),
			(SELECT count(DISTINCT v.customer_id) FROM visits v WHERE v.store_id = $1 AND v.employee_id = u.id AND `+visitRange+`),
			(SELECT count(*) FROM customers c WHERE c.store_id = $1 AND c.created_by = u.id AND `+customerRange+`),
			(SELECT count(*) FROM opportunities o WHERE o.store_id = $1 AND o.employee_id = u.id AND `+oppRange+`),
			(SELECT count(*) FROM opportunity_stage_events e JOIN opportunities o ON o.id = e.opportunity_id WHERE e.store_id = $1 AND o.employee_id = u.id AND e.to_stage = 'offer' AND `+eventRange+`),
			(SELECT count(*) FROM opportunity_stage_events e JOIN opportunities o ON o.id = e.opportunity_id WHERE e.store_id = $1 AND o.employee_id = u.id AND e.to_stage = 'won' AND `+eventRange+`),
			(SELECT count(*) FROM customers c WHERE c.store_id = $1 AND c.owner_id = u.id),
			(SELECT count(*) FROM opportunities o WHERE o.store_id = $1 AND o.employee_id = u.id AND o.stage NOT IN ('won', 'lost')),
			(SELECT count(*) FROM follow_ups f WHERE f.store_id = $1 AND f.employee_id = u.id AND f.status <> 'done'),
			(SELECT count(*) FROM follow_ups f WHERE f.store_id = $1 AND f.employee_id = u.id AND f.status <> 'done' AND f.due < $2::date)
		FROM users u WHERE u.store_id = $1 ORDER BY u.name`, a...)
	if err != nil {
		return nil, mapErr(err)
	}
	items, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (crm.EmployeeCounts, error) {
		var e crm.EmployeeCounts
		return e, r.Scan(&e.EmployeeID, &e.Visits, &e.CustomersHandled, &e.NewCustomers, &e.OpportunitiesCreated, &e.Offers, &e.Won, &e.PortfolioCustomers, &e.ActiveOpportunities, &e.OpenFollowUps, &e.OverdueFollowUps)
	})
	return items, mapErr(err)
}

// ---- Administration (CLI) ----

func (q *queries) InsertStore(ctx context.Context, s crm.RetailStore) error {
	_, err := q.q.Exec(ctx, `INSERT INTO stores (id, name, timezone) VALUES ($1, $2, $3)`, s.ID, s.Name, s.Timezone)
	return mapErr(err)
}

func (q *queries) StoreByName(ctx context.Context, name string) (crm.RetailStore, error) {
	var s crm.RetailStore
	err := q.q.QueryRow(ctx, `SELECT id::text, name, timezone FROM stores WHERE name = $1 ORDER BY created_at LIMIT 1`, name).Scan(&s.ID, &s.Name, &s.Timezone)
	return s, mapErr(err)
}

func (q *queries) Stores(ctx context.Context) ([]crm.RetailStore, error) {
	rows, err := q.q.Query(ctx, `SELECT id::text, name, timezone FROM stores ORDER BY created_at`)
	if err != nil {
		return nil, mapErr(err)
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (crm.RetailStore, error) {
		var s crm.RetailStore
		return s, r.Scan(&s.ID, &s.Name, &s.Timezone)
	})
}
