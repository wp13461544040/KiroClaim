package handler

import (
	"time"

	"github.com/huey1in/KiroClaim/database"
	"github.com/huey1in/KiroClaim/model"

	"gorm.io/gorm"
)

const (
	cardStatusUnused = "unused"
	cardStatusActive = "active"
)

func cardStatusFromUsedAt(usedAt *time.Time) string {
	if usedAt == nil {
		return cardStatusUnused
	}
	return cardStatusActive
}

func cardIsUsed(card *model.Card) bool {
	return card != nil && card.UsedAt != nil
}

func cardBindings(cardID uint) ([]model.Account, error) {
	accounts, _, err := cardBindingAccounts(cardID)
	return accounts, err
}

func cardBindingAccounts(cardID uint) ([]model.Account, int, error) {
	var bindings []model.CardAccount
	if err := database.DB.Where("card_id = ?", cardID).Order("id asc").Find(&bindings).Error; err != nil {
		return nil, 0, err
	}
	if len(bindings) == 0 {
		return nil, 0, nil
	}

	ids := make([]uint, 0, len(bindings))
	for _, binding := range bindings {
		ids = append(ids, binding.AccountID)
	}

	var accounts []model.Account
	if err := database.DB.Where("id IN ?", ids).Find(&accounts).Error; err != nil {
		return nil, 0, err
	}

	accountByID := make(map[uint]model.Account, len(accounts))
	for _, account := range accounts {
		accountByID[account.ID] = account
	}

	ordered := make([]model.Account, 0, len(accounts))
	missing := 0
	for _, binding := range bindings {
		account, ok := accountByID[binding.AccountID]
		if !ok {
			missing++
			continue
		}
		ordered = append(ordered, account)
	}
	return ordered, missing, nil
}

func releaseAccountsForCards(cardIDs []uint) error {
	if len(cardIDs) == 0 {
		return nil
	}

	var accountIDs []uint
	if err := database.DB.Model(&model.CardAccount{}).
		Where("card_id IN ?", cardIDs).
		Distinct().
		Pluck("account_id", &accountIDs).Error; err != nil {
		return err
	}

	if len(accountIDs) > 0 {
		if err := database.DB.Model(&model.Account{}).
			Where("id IN ?", accountIDs).
			Updates(map[string]interface{}{"used": false, "used_at": nil}).Error; err != nil {
			return err
		}
	}

	return database.DB.Where("card_id IN ?", cardIDs).Delete(&model.CardAccount{}).Error
}

func deleteAccountsPhysically(accountIDs []uint) *gorm.DB {
	if len(accountIDs) == 0 {
		return database.DB.Where("1 = 0").Delete(&model.Account{})
	}
	return database.DB.Unscoped().Where("id IN ?", accountIDs).Delete(&model.Account{})
}
