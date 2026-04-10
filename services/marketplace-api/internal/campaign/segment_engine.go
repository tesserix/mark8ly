package campaign

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SegmentQueryLimit caps the number of emails returned per segment query
// to prevent unbounded memory usage. Acceptable for M4 SMB launch;
// streaming is a follow-up.
const SegmentQueryLimit = 50000

// SegmentEngine resolves segment rules to email addresses. It queries
// customer_loyalties and orders tables.
type SegmentEngine struct {
	db *gorm.DB
}

// NewSegmentEngine constructs the engine.
func NewSegmentEngine(db *gorm.DB) *SegmentEngine {
	return &SegmentEngine{db: db}
}

// ResolveEmails parses the rules JSONB and returns matching customer
// emails for the given store. If multiple rules exist, they are ANDed
// (intersection). If rules is empty or contains a single "all" rule,
// returns all enrolled customers.
//
// MEDIUM FIX 4: all queries include tenant_id for convention consistency.
func (e *SegmentEngine) ResolveEmails(ctx context.Context, tenantID, storeID uuid.UUID, rulesJSON []byte) ([]string, error) {
	var rules []SegmentRule
	if err := json.Unmarshal(rulesJSON, &rules); err != nil {
		return nil, fmt.Errorf("segment: invalid rules JSON: %w", err)
	}

	if len(rules) == 0 {
		return e.allEnrolled(ctx, tenantID, storeID)
	}

	// Build result set — start with first rule, intersect subsequent.
	var emails []string
	for i, rule := range rules {
		var ruleEmails []string
		var err error
		switch rule.Type {
		case "all":
			ruleEmails, err = e.allEnrolled(ctx, tenantID, storeID)
		case "loyalty_tier":
			ruleEmails, err = e.byLoyaltyTier(ctx, tenantID, storeID, rule.Value)
		case "has_ordered":
			ruleEmails, err = e.hasOrdered(ctx, tenantID, storeID)
		case "inactive_days":
			days, parseErr := strconv.Atoi(rule.Value)
			if parseErr != nil {
				return nil, fmt.Errorf("segment: invalid inactive_days value %q: %w", rule.Value, parseErr)
			}
			ruleEmails, err = e.inactiveDays(ctx, tenantID, storeID, days)
		default:
			return nil, fmt.Errorf("segment: unknown rule type %q", rule.Type)
		}
		if err != nil {
			return nil, err
		}
		if i == 0 {
			emails = ruleEmails
		} else {
			emails = intersect(emails, ruleEmails)
		}
	}
	return emails, nil
}

func (e *SegmentEngine) allEnrolled(ctx context.Context, tenantID, storeID uuid.UUID) ([]string, error) {
	var emails []string
	err := e.db.WithContext(ctx).
		Raw("SELECT customer_email FROM customer_loyalties WHERE tenant_id = ? AND store_id = ? LIMIT ?",
			tenantID, storeID, SegmentQueryLimit).
		Scan(&emails).Error
	return emails, err
}

func (e *SegmentEngine) byLoyaltyTier(ctx context.Context, tenantID, storeID uuid.UUID, tier string) ([]string, error) {
	var emails []string
	err := e.db.WithContext(ctx).
		Raw("SELECT customer_email FROM customer_loyalties WHERE tenant_id = ? AND store_id = ? AND tier = ? LIMIT ?",
			tenantID, storeID, tier, SegmentQueryLimit).
		Scan(&emails).Error
	return emails, err
}

func (e *SegmentEngine) hasOrdered(ctx context.Context, tenantID, storeID uuid.UUID) ([]string, error) {
	var emails []string
	err := e.db.WithContext(ctx).
		Raw("SELECT DISTINCT customer_email FROM orders WHERE tenant_id = ? AND store_id = ? AND status != 'cancelled' LIMIT ?",
			tenantID, storeID, SegmentQueryLimit).
		Scan(&emails).Error
	return emails, err
}

func (e *SegmentEngine) inactiveDays(ctx context.Context, tenantID, storeID uuid.UUID, days int) ([]string, error) {
	cutoff := time.Now().AddDate(0, 0, -days)
	// Customers enrolled but whose last order is before cutoff (or no order).
	var emails []string
	err := e.db.WithContext(ctx).
		Raw(`SELECT cl.customer_email FROM customer_loyalties cl
			WHERE cl.tenant_id = ? AND cl.store_id = ?
			AND cl.customer_email NOT IN (
				SELECT DISTINCT customer_email FROM orders
				WHERE tenant_id = ? AND store_id = ? AND status != 'cancelled' AND placed_at > ?
			)
			LIMIT ?`, tenantID, storeID, tenantID, storeID, cutoff, SegmentQueryLimit).
		Scan(&emails).Error
	return emails, err
}

// intersect returns elements present in both slices.
func intersect(a, b []string) []string {
	set := make(map[string]struct{}, len(b))
	for _, v := range b {
		set[v] = struct{}{}
	}
	var result []string
	for _, v := range a {
		if _, ok := set[v]; ok {
			result = append(result, v)
		}
	}
	return result
}
