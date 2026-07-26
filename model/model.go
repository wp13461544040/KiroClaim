package model

import (
	"time"

	"github.com/huey1in/KiroClaim/utils"

	"gorm.io/gorm"
)

type AccountStatus string

const (
	AccountStatusActive    AccountStatus = "active"
	AccountStatusSuspended AccountStatus = "suspended"
)

const (
	AccountProviderIDC    = "idc"
	AccountProviderSocial = "social"
)

type Account struct {
	gorm.Model
	AccessToken   string `gorm:"type:text;not null"`
	RefreshToken  string `gorm:"type:text;not null"`
	ClientId      string `gorm:"type:text"`
	ClientSecret  string `gorm:"type:text"`
	Provider      string `gorm:"type:varchar(50)"`
	Region        string `gorm:"type:varchar(50)"`
	Used          bool   `gorm:"default:false"`
	UsedAt        *time.Time
	Status        AccountStatus `gorm:"type:varchar(20)"`
	LastCheckedAt *time.Time
	Email         string  `gorm:"type:varchar(255)"`
	Subscription  string  `gorm:"type:varchar(50)"`
	CreditUsed    float64 `gorm:"default:0"`
	CreditLimit   float64 `gorm:"default:0"`
}

func (a *Account) BeforeCreate(tx *gorm.DB) error {
	if a.Provider == "" {
		if a.ClientId != "" && a.ClientSecret != "" {
			a.Provider = AccountProviderIDC
		} else {
			a.Provider = AccountProviderSocial
		}
	}
	if a.Region == "" {
		a.Region = "us-east-1"
	}
	if a.Status == "" {
		a.Status = AccountStatusActive
	}
	return nil
}

func (a *Account) BeforeSave(tx *gorm.DB) error {
	a.AccessToken = utils.Encrypt(a.AccessToken)
	a.RefreshToken = utils.Encrypt(a.RefreshToken)
	a.ClientSecret = utils.Encrypt(a.ClientSecret)
	return nil
}

func (a *Account) AfterFind(tx *gorm.DB) error {
	a.AccessToken = utils.Decrypt(a.AccessToken)
	a.RefreshToken = utils.Decrypt(a.RefreshToken)
	a.ClientSecret = utils.Decrypt(a.ClientSecret)
	return nil
}

type KV struct {
	Key   string `gorm:"primaryKey;type:varchar(255)"`
	Value string `gorm:"type:text"`
}

const (
	KVJWTSecret       = "runtime.jwt_secret"
	KVEncryptionKey   = "runtime.encryption_key"
	KVRuntimeSettings = "settings.runtime"
	KVCommerceSettings = "settings.commerce"
)

type Card struct {
	gorm.Model
	Code         string `gorm:"type:varchar(255);uniqueIndex;not null"`
	UsedAt       *time.Time
	AccountCount int    `gorm:"default:1"`
	Subscription string `gorm:"type:varchar(50);default:''"`
}

type CardAccount struct {
	ID        uint `gorm:"primaryKey"`
	CardID    uint `gorm:"index;uniqueIndex:idx_card_account;not null"`
	AccountID uint `gorm:"index;uniqueIndex:idx_card_account;not null"`
	CreatedAt time.Time
}

type OpLog struct {
	ID        uint      `gorm:"primaryKey"`
	Action    string    `gorm:"type:varchar(50);not null;index:idx_action_created"`
	Detail    string    `gorm:"type:text"`
	Operator  string    `gorm:"type:varchar(50)"`
	ClientIP  string    `gorm:"type:varchar(45)"`
	UserAgent string    `gorm:"type:varchar(255)"`
	CreatedAt time.Time `gorm:"index:idx_action_created"`
}

type CardLog struct {
	ID        uint   `gorm:"primaryKey"`
	CardID    uint   `gorm:"index;not null"`
	Code      string `gorm:"type:varchar(255)"`
	Action    string `gorm:"type:varchar(20)"`
	AccountID uint
	Email     string `gorm:"type:varchar(255)"`
	ClientIP  string `gorm:"type:varchar(50)"`
	CreatedAt time.Time
}

type User struct {
	gorm.Model
	Username     string `gorm:"uniqueIndex;size:64;not null"`
	PasswordHash string `gorm:"type:varchar(120);not null"`
	TokenVersion int    `gorm:"default:1"`
}
