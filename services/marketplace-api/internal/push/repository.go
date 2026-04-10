package push

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Upsert(t *Token) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "device_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"token", "platform", "store_id", "updated_at"}),
	}).Create(t).Error
}

func (r *Repository) Delete(userID, tokenID uuid.UUID) error {
	return r.db.Where("id = ? AND user_id = ?", tokenID, userID).Delete(&Token{}).Error
}

func (r *Repository) DeleteByToken(tokenStr string) error {
	return r.db.Where("token = ?", tokenStr).Delete(&Token{}).Error
}

func (r *Repository) ListByStore(storeID uuid.UUID) ([]Token, error) {
	var tokens []Token
	err := r.db.Where("store_id = ?", storeID).Find(&tokens).Error
	return tokens, err
}

func (r *Repository) DeleteAllForUser(userID uuid.UUID) error {
	return r.db.Where("user_id = ?", userID).Delete(&Token{}).Error
}
