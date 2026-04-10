package admin

import (
	"testing"

	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/product"
)

func TestRequiredRoleForAction_Delete_RequiresOwner(t *testing.T) {
	got := requiredRoleForAction(product.BulkActionDelete)
	if got != authz.RoleOwner {
		t.Errorf("expected delete to require owner, got %s", got)
	}
}

func TestRequiredRoleForAction_Archive_RequiresAdmin(t *testing.T) {
	got := requiredRoleForAction(product.BulkActionArchive)
	if got != authz.RoleAdmin {
		t.Errorf("expected archive to require admin, got %s", got)
	}
}

func TestRequiredRoleForAction_Publish_RequiresAdmin(t *testing.T) {
	got := requiredRoleForAction(product.BulkActionPublish)
	if got != authz.RoleAdmin {
		t.Errorf("expected publish to require admin, got %s", got)
	}
}

func TestRequiredRoleForAction_AssignCategory_RequiresAdmin(t *testing.T) {
	got := requiredRoleForAction(product.BulkActionAssignCategory)
	if got != authz.RoleAdmin {
		t.Errorf("expected assign_category to require admin, got %s", got)
	}
}
