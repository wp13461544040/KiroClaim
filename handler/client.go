package handler

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/huey1in/KiroClaim/database"
	"github.com/huey1in/KiroClaim/model"
	"github.com/huey1in/KiroClaim/utils"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"gorm.io/gorm"
)

// POST /api/activate
// Body: { "code": "xxx" }
func Activate(c *gin.Context) {
	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": err.Error()})
		return
	}

	var card model.Card
	if err := database.DB.Where("code = ?", req.Code).First(&card).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 1, "message": "卡密不存在"})
		return
	}
	if cardIsUsed(&card) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "卡密已被使用"})
		return
	}

	now := time.Now()
	if card.AccountCount > 1 {
		accounts, err := popMultipleAccounts(card.AccountCount, card.Subscription)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"code": 2, "message": "账号不足，请联系管理员补充"})
			return
		}
		result := database.DB.Model(&model.Card{}).Where("id = ? AND used_at IS NULL", card.ID).Update("used_at", now)
		if result.RowsAffected == 0 {
			for _, acc := range accounts {
				database.DB.Model(acc).Update("used", false)
			}
			c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "卡密已被使用"})
			return
		}

		accountResps := make([]gin.H, 0, len(accounts))
		accountIDStrs := make([]string, 0, len(accounts))
		for _, acc := range accounts {
			database.DB.Create(&model.CardAccount{CardID: card.ID, AccountID: acc.ID})
			database.DB.Model(acc).Updates(map[string]interface{}{"used": true, "used_at": now})
			accountResps = append(accountResps, buildAccountResp(acc))
			accountIDStrs = append(accountIDStrs, strconv.Itoa(int(acc.ID)))
			database.DB.Create(&model.CardLog{
				CardID:    card.ID,
				Code:      card.Code,
				Action:    "activate",
				AccountID: acc.ID,
				Email:     acc.Email,
				ClientIP:  c.ClientIP(),
			})
		}

		AddOpLogWithCtx(c, "activate", "多号卡激活 "+req.Code+"，绑定 "+strconv.Itoa(len(accounts))+" 个账号 ID:["+strings.Join(accountIDStrs, ",")+"], IP: "+c.ClientIP(), "client")
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "激活成功", "data": gin.H{"accounts": accountResps, "account_count": len(accounts)}})
		return
	}

	account, err := popAccount(0, card.Subscription)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 2, "message": "剩余账号不足，请联系管理员补充"})
		return
	}

	updates := map[string]interface{}{
		"used_at": now,
	}
	result := database.DB.Model(&model.Card{}).Where("id = ? AND used_at IS NULL", card.ID).Updates(updates)
	if result.RowsAffected == 0 {
		database.DB.Model(account).Update("used", false)
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "卡密已被使用"})
		return
	}
	database.DB.Create(&model.CardAccount{CardID: card.ID, AccountID: account.ID})
	database.DB.Model(account).Updates(map[string]interface{}{"used": true, "used_at": now})

	AddOpLogWithCtx(c, "activate", "激活卡密 "+req.Code+"，绑定账号 ID:"+strconv.Itoa(int(account.ID))+", IP: "+c.ClientIP(), "client")
	database.DB.Create(&model.CardLog{
		CardID:    card.ID,
		Code:      card.Code,
		Action:    "activate",
		AccountID: account.ID,
		Email:     account.Email,
		ClientIP:  c.ClientIP(),
	})
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "激活成功", "data": gin.H{"account": buildAccountResp(account)}})
}

// GET /api/status?code=xxx
func Status(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "code 不能为空"})
		return
	}

	var card model.Card
	if err := database.DB.Where("code = ?", code).First(&card).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 1, "message": "卡密不存在"})
		return
	}

	active := cardIsUsed(&card)
	resp := gin.H{"active": active}
	if active {
		accounts, err := cardBindings(card.ID)
		if err == nil && len(accounts) > 0 {
			if len(accounts) == 1 {
				resp["account"] = buildAccountResp(&accounts[0])
			} else {
				items := make([]gin.H, 0, len(accounts))
				for i := range accounts {
					items = append(items, buildAccountResp(&accounts[i]))
				}
				resp["accounts"] = items
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": resp})
}

func normalizeSubscription(subscription string) string {
	return strings.TrimSpace(subscription)
}

func subscriptionMatches(actual string, required string) bool {
	required = normalizeSubscription(required)
	if required == "" {
		return true
	}
	return strings.TrimSpace(actual) == required
}

func isDispatchable(acc *model.Account, subscription string) bool {
	if acc == nil {
		return false
	}
	if acc.Used || acc.Status != model.AccountStatusActive {
		return false
	}
	if acc.CreditUsed != 0 {
		return false
	}
	if acc.AccessToken == "" {
		return false
	}
	return subscriptionMatches(acc.Subscription, subscription)
}

func filterAccountSubscriptionQuery(q *gorm.DB, subscription string) *gorm.DB {
	subscription = normalizeSubscription(subscription)
	if subscription == "" {
		return q
	}
	return q.Where("subscription = ?", subscription)
}

func dispatchHealthCheckEnabled() bool {
	return GetCurrentSettings().DispatchHealthCheckEnabled
}

func popAccount(excludeID uint, subscription string) (*model.Account, error) {
	timer := prometheus.NewTimer(utils.DispatchDuration)
	defer timer.ObserveDuration()

	q := database.DB.Where("used = ?", false).
		Where("status = ?", model.AccountStatusActive).
		Where("credit_used = ?", 0)
	if excludeID > 0 {
		q = q.Where("id != ?", excludeID)
	}
	q = filterAccountSubscriptionQuery(q, subscription)

	var candidates []model.Account
	if err := q.Order("created_at ASC, id ASC").Limit(50).Find(&candidates).Error; err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	if !dispatchHealthCheckEnabled() {
		for i := range candidates {
			if isDispatchable(&candidates[i], subscription) {
				account := candidates[i]
				return &account, nil
			}
		}
		return nil, gorm.ErrRecordNotFound
	}

	freshCutoff := time.Now().Add(-20 * time.Minute)
	for _, account := range candidates {
		if account.LastCheckedAt == nil || !account.LastCheckedAt.After(freshCutoff) {
			continue
		}
		if !isDispatchable(&account, subscription) {
			continue
		}
		if !verifyDispatchable(account.AccessToken) {
			continue
		}
		acc := account
		return &acc, nil
	}

	unchecked := make([]model.Account, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.LastCheckedAt != nil && candidate.LastCheckedAt.After(freshCutoff) {
			continue
		}
		unchecked = append(unchecked, candidate)
	}
	if len(unchecked) == 0 {
		return nil, gorm.ErrRecordNotFound
	}

	type checkResult struct {
		account model.Account
	}

	winner := make(chan *checkResult, 1)
	done := make(chan struct{})
	localLimit := currentUpstreamCheckConcurrency()
	if localLimit <= 0 {
		localLimit = 6
	}
	sem := make(chan struct{}, localLimit)
	var found atomic.Bool

	go func() {
		defer close(done)
		var wg sync.WaitGroup
		for i := range unchecked {
			if found.Load() {
				break
			}
			sem <- struct{}{}
			wg.Add(1)
			go func(acc model.Account) {
				defer func() { <-sem; wg.Done() }()
				if found.Load() {
					return
				}
				r := checkAccountHealth(acc)
				updates := buildHealthUpdates(r, time.Now())
				if err := persistHealthUpdates(acc.ID, updates); err != nil {
					return
				}
				if r.errMsg != "" {
					return
				}
				acc.AccessToken = r.newToken
				acc.RefreshToken = r.newRefresh
				acc.Email = r.email
				acc.Subscription = r.subscription
				acc.CreditUsed = r.creditUsed
				acc.CreditLimit = r.creditLimit
				if r.provider != "" {
					acc.Provider = r.provider
				}
				acc.Status = r.status
				if !isDispatchable(&acc, subscription) {
					return
				}
				if found.CompareAndSwap(false, true) {
					winner <- &checkResult{account: acc}
				}
			}(unchecked[i])
		}
		wg.Wait()
	}()

	select {
	case w := <-winner:
		acc := w.account
		return &acc, nil
	case <-done:
		return nil, gorm.ErrRecordNotFound
	}
}

func buildTokenEntry(a *model.Account) gin.H {
	return gin.H{
		"accessToken":  a.AccessToken,
		"clientId":     a.ClientId,
		"clientSecret": a.ClientSecret,
		"creditLimit":  a.CreditLimit,
		"creditUsed":   a.CreditUsed,
		"email":        a.Email,
		"provider":     a.Provider,
		"refreshToken": a.RefreshToken,
		"region":       a.Region,
		"subscription": a.Subscription,
	}
}

func buildTokenArray(a *model.Account) []gin.H {
	return []gin.H{buildTokenEntry(a)}
}

func buildMultiTokenArray(accounts []model.Account) []gin.H {
	result := make([]gin.H, 0, len(accounts))
	for i := range accounts {
		result = append(result, buildTokenEntry(&accounts[i]))
	}
	return result
}

func popMultipleAccounts(n int, subscription string) ([]*model.Account, error) {
	if n <= 0 {
		return nil, gorm.ErrRecordNotFound
	}

	q := database.DB.Model(&model.Account{}).Where("used = ?", false).
		Where("status = ?", model.AccountStatusActive).
		Where("credit_used = ?", 0)
	q = filterAccountSubscriptionQuery(q, subscription)

	var available int64
	q.Count(&available)
	if int(available) < n {
		return nil, gorm.ErrRecordNotFound
	}

	var candidates []model.Account
	if err := q.Order("created_at ASC, id ASC").Limit(n * 4).Find(&candidates).Error; err != nil {
		return nil, err
	}
	if !dispatchHealthCheckEnabled() {
		accounts := make([]*model.Account, 0, n)
		for i := range candidates {
			if !isDispatchable(&candidates[i], subscription) {
				continue
			}
			account := candidates[i]
			accounts = append(accounts, &account)
			if len(accounts) == n {
				return accounts, nil
			}
		}
		return nil, gorm.ErrRecordNotFound
	}

	accounts := make([]*model.Account, 0, n)
	freshCutoff := time.Now().Add(-20 * time.Minute)
	for i := range candidates {
		if len(accounts) >= n {
			break
		}
		candidate := candidates[i]
		if candidate.LastCheckedAt == nil || !candidate.LastCheckedAt.After(freshCutoff) {
			continue
		}
		if !isDispatchable(&candidate, subscription) {
			continue
		}
		if !verifyDispatchable(candidate.AccessToken) {
			continue
		}
		acc := candidate
		accounts = append(accounts, &acc)
	}

	for i := range candidates {
		if len(accounts) >= n {
			break
		}
		candidate := candidates[i]
		if candidate.LastCheckedAt != nil && candidate.LastCheckedAt.After(freshCutoff) {
			continue
		}
		result := checkAccountHealth(candidate)
		updates := buildHealthUpdates(result, time.Now())
		if err := persistHealthUpdates(candidate.ID, updates); err != nil {
			continue
		}
		if result.errMsg != "" {
			continue
		}

		candidate.AccessToken = result.newToken
		candidate.RefreshToken = result.newRefresh
		candidate.Email = result.email
		candidate.Subscription = result.subscription
		candidate.CreditUsed = result.creditUsed
		candidate.CreditLimit = result.creditLimit
		if result.provider != "" {
			candidate.Provider = result.provider
		}
		candidate.Status = result.status
		if !isDispatchable(&candidate, subscription) {
			continue
		}
		if !verifyDispatchable(candidate.AccessToken) {
			continue
		}
		acc := candidate
		accounts = append(accounts, &acc)
	}

	if len(accounts) < n {
		return nil, gorm.ErrRecordNotFound
	}
	return accounts, nil
}

func buildAccountResp(a *model.Account) gin.H {
	return gin.H{
		"accessToken":  a.AccessToken,
		"refreshToken": a.RefreshToken,
		"clientId":     a.ClientId,
		"clientSecret": a.ClientSecret,
		"provider":     a.Provider,
		"region":       a.Region,
	}
}

// GET /token/:code
func GetToken(c *gin.Context) {
	codeParam := c.Param("code")
	codes := parseTokenCodes(codeParam)
	if len(codes) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "卡密不能为空"})
		return
	}
	if c.Query("stream") == "1" {
		streamToken(c, codes)
		return
	}

	allTokens := make([]gin.H, 0, len(codes))
	for _, code := range codes {
		tokens, errResp, status := processOneCode(c, code)
		if errResp != nil {
			c.JSON(status, errResp)
			return
		}
		allTokens = append(allTokens, tokens...)
	}

	c.JSON(http.StatusOK, allTokens)
}

func parseTokenCodes(codeParam string) []string {
	codeParam = strings.ReplaceAll(codeParam, "，", ",")
	parts := strings.Split(codeParam, ",")
	seen := make(map[string]bool)
	codes := make([]string, 0, len(parts))
	for _, part := range parts {
		code := strings.TrimSpace(part)
		if code != "" && !seen[code] {
			seen[code] = true
			codes = append(codes, code)
		}
	}
	return codes
}

type tokenStreamCard struct {
	code  string
	card  model.Card
	total int
	used  bool
}

func streamToken(c *gin.Context, codes []string) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	cards, errBody, status := loadTokenStreamCards(codes)
	if errBody != nil {
		streamTokenEvent(c, "fail", gin.H{"status": status, "message": errBody["message"]})
		return
	}

	total := 0
	for _, card := range cards {
		total += card.total
	}
	if total <= 0 {
		streamTokenEvent(c, "fail", gin.H{"status": http.StatusServiceUnavailable, "message": "没有可提取的账号"})
		return
	}

	if !streamTokenEvent(c, "init", gin.H{"total": total}) {
		return
	}

	index := 0
	for _, card := range cards {
		if ok := streamOneTokenCode(c, card, &index); !ok {
			return
		}
	}
	streamTokenEvent(c, "done", gin.H{"total": index})
}

func loadTokenStreamCards(codes []string) ([]tokenStreamCard, gin.H, int) {
	result := make([]tokenStreamCard, 0, len(codes))
	for _, code := range codes {
		var card model.Card
		if err := database.DB.Where("code = ?", code).First(&card).Error; err != nil {
			return nil, gin.H{"code": 1, "message": "卡密不存在: " + code}, http.StatusNotFound
		}

		item := tokenStreamCard{code: code, card: card, used: cardIsUsed(&card)}
		item.total = card.AccountCount
		if item.total <= 0 {
			item.total = 1
		}
		result = append(result, item)
	}
	return result, nil, 0
}

func streamOneTokenCode(c *gin.Context, item tokenStreamCard, index *int) bool {
	if item.used {
		return streamUsedTokenCode(c, item, index)
	}
	return streamFreshTokenCode(c, item, index)
}

func streamUsedTokenCode(c *gin.Context, item tokenStreamCard, index *int) bool {
	accounts, missing, err := cardBindingAccounts(item.card.ID)
	if err != nil {
		streamTokenEvent(c, "fail", gin.H{"status": http.StatusInternalServerError, "message": "绑定账号异常: " + item.code})
		return false
	}
	if missing > 0 || len(accounts) == 0 {
		streamTokenEvent(c, "fail", gin.H{
			"status":  http.StatusGone,
			"message": "卡密绑定的账号已经删档，无法再次提取: " + item.code,
			"reason":  "account_deleted",
		})
		return false
	}
	sent := 0
	for i := range accounts {
		account := accounts[i]
		if !streamAccountToken(c, *index, item.code, &account) {
			return false
		}
		*index = *index + 1
		sent++
	}
	if sent >= item.total {
		return true
	}
	ok, _ := streamFillTokenAccounts(c, item, item.total-sent, index)
	return ok
}

func streamFreshTokenCode(c *gin.Context, item tokenStreamCard, index *int) bool {
	now := time.Now()
	claimed := database.DB.Model(&model.Card{}).
		Where("id = ? AND used_at IS NULL", item.card.ID).
		Update("used_at", now)
	if claimed.Error != nil {
		streamTokenEvent(c, "fail", gin.H{"status": http.StatusInternalServerError, "message": "卡密状态更新失败: " + item.code})
		return false
	}
	if claimed.RowsAffected == 0 {
		var fresh model.Card
		if err := database.DB.First(&fresh, item.card.ID).Error; err != nil {
			streamTokenEvent(c, "fail", gin.H{"status": http.StatusBadRequest, "message": "卡密已被使用: " + item.code})
			return false
		}
		item.card = fresh
		item.used = true
		return streamUsedTokenCode(c, item, index)
	}

	ok, sent := streamFillTokenAccounts(c, item, item.total, index)
	if !ok && sent == 0 {
		database.DB.Model(&model.Card{}).Where("id = ?", item.card.ID).Update("used_at", nil)
	}
	return ok
}

func streamFillTokenAccounts(c *gin.Context, item tokenStreamCard, needed int, index *int) (bool, int) {
	if needed <= 0 {
		return true, 0
	}
	clientIP := c.ClientIP()
	now := time.Now()
	sent := 0
	accountIDStrs := make([]string, 0, needed)
	for sent < needed {
		account, err := popAccount(0, item.card.Subscription)
		if err != nil {
			streamTokenEvent(c, "fail", gin.H{
				"status":  http.StatusServiceUnavailable,
				"message": "账号池账号不足: " + item.code,
				"reason":  "account_pool_shortage",
				"partial": sent,
			})
			return false, sent
		}

		reserved := database.DB.Model(&model.Account{}).
			Where("id = ? AND used = ?", account.ID, false).
			Updates(map[string]interface{}{"used": true, "used_at": now})
		if reserved.Error != nil || reserved.RowsAffected == 0 {
			continue
		}
		if err := database.DB.Create(&model.CardAccount{CardID: item.card.ID, AccountID: account.ID}).Error; err != nil {
			database.DB.Model(account).Updates(map[string]interface{}{"used": false, "used_at": nil})
			continue
		}

		accountIDStrs = append(accountIDStrs, strconv.Itoa(int(account.ID)))
		database.DB.Create(&model.CardLog{CardID: item.card.ID, Code: item.card.Code, Action: "activate", AccountID: account.ID, Email: account.Email, ClientIP: clientIP})
		if !streamAccountToken(c, *index, item.code, account) {
			return false, sent
		}
		*index = *index + 1
		sent++
	}

	if needed > 1 {
		AddOpLogWithCtx(c, "activate", "多号卡流式补号 "+item.code+"，绑定 "+strconv.Itoa(sent)+" 个账号 ID:["+strings.Join(accountIDStrs, ",")+"], IP: "+clientIP, "client")
	} else if len(accountIDStrs) > 0 {
		AddOpLogWithCtx(c, "activate", "凭证接口流式激活卡密 "+item.code+"，绑定账号 ID:"+accountIDStrs[0]+", IP: "+clientIP, "client")
	}
	return true, sent
}

func streamAccountToken(c *gin.Context, index int, code string, account *model.Account) bool {
	return streamTokenEvent(c, "account", gin.H{
		"index":   index,
		"code":    code,
		"account": buildTokenEntry(account),
	})
}

func streamTokenEvent(c *gin.Context, event string, data gin.H) bool {
	select {
	case <-c.Request.Context().Done():
		return false
	default:
	}
	c.SSEvent(event, data)
	c.Writer.Flush()
	return true
}

func processOneCode(c *gin.Context, code string) ([]gin.H, gin.H, int) {
	clientIP := c.ClientIP()
	var card model.Card
	if err := database.DB.Where("code = ?", code).First(&card).Error; err != nil {
		return nil, gin.H{"code": 1, "message": "卡密不存在: " + code}, http.StatusNotFound
	}

	isMulti := card.AccountCount > 1
	if cardIsUsed(&card) {
		accounts, missing, err := cardBindingAccounts(card.ID)
		if err != nil {
			return nil, gin.H{"code": 1, "message": "绑定账号异常: " + code}, http.StatusInternalServerError
		}
		if missing > 0 || len(accounts) == 0 {
			return nil, gin.H{"code": 2, "message": "卡密绑定的账号已经删档，无法再次提取: " + code, "reason": "account_deleted"}, http.StatusGone
		}
		if len(accounts) == 1 {
			return buildTokenArray(&accounts[0]), nil, 0
		}
		return buildMultiTokenArray(accounts), nil, 0
	}

	now := time.Now()
	if isMulti {
		accounts, err := popMultipleAccounts(card.AccountCount, card.Subscription)
		if err != nil {
			return nil, gin.H{"code": 2, "message": "账号池账号不足: " + code}, http.StatusServiceUnavailable
		}
		result := database.DB.Model(&model.Card{}).Where("id = ? AND used_at IS NULL", card.ID).Update("used_at", now)
		if result.RowsAffected == 0 {
			for _, acc := range accounts {
				database.DB.Model(acc).Update("used", false)
			}
			return nil, gin.H{"code": 1, "message": "卡密已被使用: " + code}, http.StatusBadRequest
		}

		idStrs := make([]string, 0, len(accounts))
		mods := make([]model.Account, 0, len(accounts))
		for _, acc := range accounts {
			database.DB.Create(&model.CardAccount{CardID: card.ID, AccountID: acc.ID})
			database.DB.Model(acc).Updates(map[string]interface{}{"used": true, "used_at": now})
			idStrs = append(idStrs, strconv.Itoa(int(acc.ID)))
			mods = append(mods, *acc)
			database.DB.Create(&model.CardLog{CardID: card.ID, Code: card.Code, Action: "activate", AccountID: acc.ID, Email: acc.Email, ClientIP: clientIP})
		}
		AddOpLogWithCtx(c, "activate", "多号卡激活 "+code+"，绑定 "+strconv.Itoa(len(accounts))+" 个账号 ID:["+strings.Join(idStrs, ",")+"], IP: "+clientIP, "client")
		return buildMultiTokenArray(mods), nil, 0
	}

	account, err := popAccount(0, card.Subscription)
	if err != nil {
		return nil, gin.H{"code": 2, "message": "账号池已空: " + code}, http.StatusServiceUnavailable
	}
	result := database.DB.Model(&model.Card{}).Where("id = ? AND used_at IS NULL", card.ID).Update("used_at", now)
	if result.RowsAffected == 0 {
		database.DB.Model(account).Update("used", false)
		return nil, gin.H{"code": 1, "message": "卡密已被使用: " + code}, http.StatusBadRequest
	}
	database.DB.Create(&model.CardAccount{CardID: card.ID, AccountID: account.ID})
	database.DB.Model(account).Updates(map[string]interface{}{"used": true, "used_at": now})
	AddOpLogWithCtx(c, "activate", "凭证接口激活卡密 "+code+"，绑定账号 ID:"+strconv.Itoa(int(account.ID))+", IP: "+clientIP, "client")
	database.DB.Create(&model.CardLog{CardID: card.ID, Code: card.Code, Action: "activate", AccountID: account.ID, Email: account.Email, ClientIP: clientIP})
	return buildTokenArray(account), nil, 0
}

func checkBoundAccountsForToken(code string, accounts []model.Account) ([]model.Account, gin.H, int) {
	checked := make([]model.Account, 0, len(accounts))
	for i := range accounts {
		account := accounts[i]
		checkedAccount, errBody, status := checkBoundAccountForToken(code, &account)
		if errBody != nil {
			return nil, errBody, status
		}
		checked = append(checked, *checkedAccount)
	}
	return checked, nil, 0
}

func checkBoundAccountForToken(code string, account *model.Account) (*model.Account, gin.H, int) {
	result := checkAccountHealth(*account)
	if err := applyHealthResult(account.ID, result); err != nil {
		return nil, gin.H{"code": 1, "message": "账号检查结果写入失败: " + err.Error()}, http.StatusInternalServerError
	}
	if result.status != model.AccountStatusActive || result.errMsg != "" {
		return nil, gin.H{
			"code":    2,
			"message": "绑定账号不可用，请联系管理员处理: " + code,
			"reason":  result.errMsg,
		}, http.StatusServiceUnavailable
	}
	var fresh model.Account
	if err := database.DB.First(&fresh, account.ID).Error; err != nil {
		return nil, gin.H{"code": 1, "message": "绑定账号异常: " + code}, http.StatusInternalServerError
	}
	return &fresh, nil, 0
}
