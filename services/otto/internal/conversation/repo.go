package conversation

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ErrNotFound is returned when a lookup has a valid scope but no row matches.
var ErrNotFound = errors.New("conversation: not found")

// Repository persists and queries conversations with mandatory tenant+store
// scoping. No method on this type will ever cross a tenant boundary — every
// read and write filter includes tenant_id + store_id.
type Repository struct {
	coll *mongo.Collection
}

func NewRepository(coll *mongo.Collection) *Repository { return &Repository{coll: coll} }

// Insert creates a new pending conversation.
func (r *Repository) Insert(ctx context.Context, c *Conversation) error {
	if c.ID == "" || c.TenantID == "" || c.StoreID == "" {
		return errors.New("conversation: id, tenant_id and store_id are required")
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	c.UpdatedAt = c.CreatedAt
	if c.LastMessageAt.IsZero() {
		c.LastMessageAt = c.CreatedAt
	}
	if c.Status == "" {
		c.Status = StatusPending
	}
	_, err := r.coll.InsertOne(ctx, c)
	return err
}

// GetByID enforces scope: a caller with tenant A cannot fetch a conversation
// belonging to tenant B even if they know the id.
func (r *Repository) GetByID(ctx context.Context, tenantID, storeID, id string) (*Conversation, error) {
	filter := bson.M{"_id": id, "tenant_id": tenantID, "store_id": storeID}
	var c Conversation
	if err := r.coll.FindOne(ctx, filter).Decode(&c); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

// GetForCustomer looks up a conversation a specific anonymous session is
// allowed to see. The session_token match is the customer-side auth check —
// without it, a malicious customer could poll arbitrary ids.
func (r *Repository) GetForCustomer(ctx context.Context, tenantID, storeID, id, sessionToken string) (*Conversation, error) {
	filter := bson.M{
		"_id":                    id,
		"tenant_id":              tenantID,
		"store_id":               storeID,
		"customer.session_token": sessionToken,
	}
	var c Conversation
	if err := r.coll.FindOne(ctx, filter).Decode(&c); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

// ListInbox returns conversations for a staff inbox, filtered by status and
// optionally by assignee. Always scoped to tenant+store.
type ListInboxParams struct {
	TenantID        string
	StoreID         string
	Status          Status // empty = all
	AssigneeUserID  string // empty = any assignee
	OnlyUnassigned  bool
	Limit           int64
}

func (r *Repository) ListInbox(ctx context.Context, p ListInboxParams) ([]Conversation, error) {
	filter := bson.M{"tenant_id": p.TenantID, "store_id": p.StoreID}
	if p.Status != "" {
		filter["status"] = p.Status
	}
	switch {
	case p.OnlyUnassigned:
		filter["assignee"] = bson.M{"$exists": false}
	case p.AssigneeUserID != "":
		filter["assignee.user_id"] = p.AssigneeUserID
	}
	limit := p.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "last_message_at", Value: -1}}).
		SetLimit(limit)
	cur, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []Conversation
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Accept assigns a staff member to the conversation and flips status to
// active. Idempotent-ish: if someone else already accepted, it returns the
// current state without error — the UI can display who's on it.
func (r *Repository) Accept(ctx context.Context, tenantID, storeID, id string, assignee Assignee) (*Conversation, error) {
	now := time.Now().UTC()
	if assignee.AssignedAt.IsZero() {
		assignee.AssignedAt = now
	}
	filter := bson.M{
		"_id":       id,
		"tenant_id": tenantID,
		"store_id":  storeID,
		"status":    StatusPending,
	}
	update := bson.M{
		"$set": bson.M{
			"status":     StatusActive,
			"assignee":   assignee,
			"updated_at": now,
		},
	}
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var c Conversation
	if err := r.coll.FindOneAndUpdate(ctx, filter, update, opts).Decode(&c); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			// Either not-our-tenant or already accepted — re-read to tell them apart.
			existing, getErr := r.GetByID(ctx, tenantID, storeID, id)
			if getErr != nil {
				return nil, getErr
			}
			return existing, nil
		}
		return nil, err
	}
	return &c, nil
}

// Close flips a conversation to closed. Both customer and staff may trigger
// this but the HTTP layer decides who's allowed.
func (r *Repository) Close(ctx context.Context, tenantID, storeID, id string) (*Conversation, error) {
	now := time.Now().UTC()
	filter := bson.M{"_id": id, "tenant_id": tenantID, "store_id": storeID}
	update := bson.M{
		"$set": bson.M{
			"status":     StatusClosed,
			"closed_at":  now,
			"updated_at": now,
		},
	}
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var c Conversation
	if err := r.coll.FindOneAndUpdate(ctx, filter, update, opts).Decode(&c); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

// BumpOnMessage updates the counters + timestamps whenever a message lands.
// The audience argument indicates who the message was *not* read by so we can
// advance the correct unread counter.
type Audience string

const (
	AudienceStaff    Audience = "staff"
	AudienceCustomer Audience = "customer"
)

func (r *Repository) BumpOnMessage(ctx context.Context, tenantID, storeID, id string, unreadFor Audience, at time.Time) error {
	filter := bson.M{"_id": id, "tenant_id": tenantID, "store_id": storeID}
	inc := bson.M{"message_count": 1}
	switch unreadFor {
	case AudienceStaff:
		inc["unread_count_staff"] = 1
	case AudienceCustomer:
		inc["unread_count_customer"] = 1
	}
	update := bson.M{
		"$set": bson.M{"last_message_at": at, "updated_at": at},
		"$inc": inc,
	}
	_, err := r.coll.UpdateOne(ctx, filter, update)
	return err
}

// ClearUnread resets the unread counter for the given audience — called when
// the corresponding UI marks the thread as read.
func (r *Repository) ClearUnread(ctx context.Context, tenantID, storeID, id string, audience Audience) error {
	filter := bson.M{"_id": id, "tenant_id": tenantID, "store_id": storeID}
	field := "unread_count_customer"
	if audience == AudienceStaff {
		field = "unread_count_staff"
	}
	_, err := r.coll.UpdateOne(ctx, filter, bson.M{"$set": bson.M{field: 0}})
	return err
}
