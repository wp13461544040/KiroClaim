package model

import (
	"gorm.io/gorm"
	"time"
)

const (
	ProductCardAvailable = "available"
	ProductCardReserved  = "reserved"
	ProductCardSold      = "sold"
	OrderPendingPayment  = "pending_payment"
	OrderPaymentReview   = "payment_review"
	OrderPaid            = "paid"
	OrderFulfilling      = "fulfilling"
	OrderCompleted       = "completed"
	OrderPaidAttention   = "paid_attention"
	OrderExpired         = "expired"
	OrderRejected        = "rejected"
	OrderRefunded        = "refunded"
)

type CommerceProduct struct {
	gorm.Model
	Name         string `gorm:"size:120;not null"`
	Description  string `gorm:"type:text"`
	ImageData    string `gorm:"type:longtext"`
	Active       bool   `gorm:"default:true;index"`
	BaseAmount   int64  `gorm:"not null"`
	BaseCurrency string `gorm:"size:12;not null"`
	Subscription string `gorm:"size:50;not null"`
	AccountCount int    `gorm:"default:1"`
}

type CommercePaymentChannel struct {
	gorm.Model
	Name          string `gorm:"size:120;not null"`
	ChannelType   string `gorm:"size:32;not null;index"`
	Active        bool   `gorm:"default:true;index"`
	SortOrder     int
	PublicConfig  string `gorm:"type:text"`
	PrivateConfig string `gorm:"type:text" json:"-"`
}

type CommerceProductCard struct {
	ID        uint   `gorm:"primaryKey"`
	ProductID uint   `gorm:"index;not null"`
	CardID    uint   `gorm:"uniqueIndex;not null"`
	OrderID   uint   `gorm:"index"`
	Status    string `gorm:"size:20;index;not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

type CommerceOrder struct {
	gorm.Model
	OrderNo           string    `gorm:"size:40;uniqueIndex;not null"`
	QueryPasswordHash string    `gorm:"size:120;not null"`
	Contact           string    `gorm:"size:255;not null"`
	ProductID         uint      `gorm:"index;not null"`
	ChannelID         uint      `gorm:"index"`
	ProductName       string    `gorm:"size:120;not null"`
	Subscription      string    `gorm:"size:50;not null"`
	Quantity          int       `gorm:"not null;default:1"`
	AccountCount      int       `gorm:"not null"`
	Amount            int64     `gorm:"not null"`
	Currency          string    `gorm:"size:12;not null"`
	Status            string    `gorm:"size:32;index;not null"`
	ProviderOrderID   string    `gorm:"size:160;index"`
	PaymentRef        string    `gorm:"size:160;index"`
	PayURL            string    `gorm:"type:text"`
	PayPayload        string    `gorm:"type:text"`
	CardID            uint      `gorm:"index"`
	ExpiresAt         time.Time `gorm:"index"`
	PaidAt            *time.Time
	CompletedAt       *time.Time
	ReviewNote        string `gorm:"type:text"`
}

type CommercePaymentProof struct {
	gorm.Model
	OrderID   uint   `gorm:"index;not null"`
	Reference string `gorm:"size:180"`
	PayerInfo string `gorm:"size:255"`
	Note      string `gorm:"type:text"`
	FilesJSON string `gorm:"type:text"`
}

type CommercePaymentEvent struct {
	ID        uint   `gorm:"primaryKey"`
	ChannelID uint   `gorm:"index;not null"`
	EventID   string `gorm:"size:160;uniqueIndex;not null"`
	OrderNo   string `gorm:"size:40;index;not null"`
	Payload   string `gorm:"type:text"`
	CreatedAt time.Time
}

type CommerceOrderLog struct {
	ID        uint   `gorm:"primaryKey"`
	OrderID   uint   `gorm:"index;not null"`
	Action    string `gorm:"size:50;not null"`
	Detail    string `gorm:"type:text"`
	Operator  string `gorm:"size:80"`
	CreatedAt time.Time
}
