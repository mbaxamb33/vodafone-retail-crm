package crm

import "vodafone/store/internal/apperr"

// Permission names a capability beyond the baseline every store member has
// (reading store customers, recording visits, managing their own work).
type Permission string

const (
	// PermAssignAnyone allows assigning follow-ups and opportunities to any store member.
	PermAssignAnyone Permission = "work.assign_any"
	// PermReturnToPool allows returning a colleague's customer to the store pool. Nobody may
	// hand a customer directly to someone else; customers are only claimed from the pool.
	PermReturnToPool Permission = "customers.return_to_pool"
	// PermManageAnyWork allows changing colleagues' follow-ups and opportunities.
	PermManageAnyWork   Permission = "work.manage_any"
	PermEditAnyNotes    Permission = "visits.edit_any_notes"
	PermEditAnyCustomer Permission = "customers.edit_any"
	PermArchiveCustomer Permission = "customers.archive"
	PermAnonymize       Permission = "customers.anonymize"
	PermViewReports     Permission = "reports.view"
	PermViewAudit       Permission = "audit.view"
	PermViewStoreWork   Permission = "work.view_store"
)

// rolePermissions is the single place that maps roles to capabilities. New roles are added here.
var rolePermissions = map[string]map[Permission]bool{
	RoleEmployee: {},
	RoleManager: {
		PermAssignAnyone: true, PermReturnToPool: true, PermManageAnyWork: true, PermEditAnyNotes: true, PermEditAnyCustomer: true,
		PermArchiveCustomer: true, PermAnonymize: true, PermViewReports: true, PermViewAudit: true, PermViewStoreWork: true,
	},
}

func ValidRole(role string) bool { _, ok := rolePermissions[role]; return ok }

func (u User) Can(p Permission) bool { return rolePermissions[u.Role][p] }

func (u User) require(p Permission) error {
	if !u.Can(p) {
		return apperr.ErrForbidden
	}
	return nil
}

// canWork reports whether u may change work (follow-ups, opportunities) owned by employeeID.
func (u User) canWork(employeeID string) bool {
	return employeeID == u.ID || u.Can(PermManageAnyWork)
}

// canEditCustomer: the owner, anyone for unowned customers, or users with PermEditAnyCustomer.
func (u User) canEditCustomer(c Customer) bool {
	return c.OwnerID == "" || c.OwnerID == u.ID || u.Can(PermEditAnyCustomer)
}
