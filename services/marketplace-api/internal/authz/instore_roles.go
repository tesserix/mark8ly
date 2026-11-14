package authz

// InStoreSellRole gates the in-store sale routes (#1103, #1104): the sale
// screen's catalogue, recording a sale, and the caller's own sales today.
// Cashiers pass; so do staff, admins and owners.
var InStoreSellRole = RelationCanSellInStore
