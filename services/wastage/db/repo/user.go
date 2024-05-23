package repo

import (
	"errors"
	"github.com/kaytu-io/kaytu-engine/services/wastage/db/connector"
	"github.com/kaytu-io/kaytu-engine/services/wastage/db/model"
	"gorm.io/gorm"
)

type UserRepo interface {
	Create(m *model.User) error
	Update(id uint, m model.User) error
	List() ([]model.User, error)
	Get(id string) (*model.User, error)
}

type UserRepoImpl struct {
	db *connector.Database
}

func NewUserRepo(db *connector.Database) UserRepo {
	return &UserRepoImpl{
		db: db,
	}
}

func (r *UserRepoImpl) Create(m *model.User) error {
	return r.db.Conn().Create(&m).Error
}

func (r *UserRepoImpl) Update(id uint, m model.User) error {
	return r.db.Conn().Model(&model.User{}).Where("user_id=?", id).Updates(&m).Error
}

func (r *UserRepoImpl) List() ([]model.User, error) {
	var ms []model.User
	tx := r.db.Conn().Model(&model.User{}).Find(&ms)
	if tx.Error != nil {
		return nil, tx.Error
	}
	return ms, nil
}

func (r *UserRepoImpl) Get(id string) (*model.User, error) {
	var m model.User
	tx := r.db.Conn().Model(&model.User{}).Where("user_id=?", id).First(&m)
	if tx.Error != nil {
		if errors.Is(tx.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, tx.Error
	}
	return &m, nil
}