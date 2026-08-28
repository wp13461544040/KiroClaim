package handler

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wp13461544040/KiroClaim/database"
	"github.com/wp13461544040/KiroClaim/model"
	"github.com/wp13461544040/KiroClaim/utils"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"gorm.io/gorm"
)

// 原子预留账号：只有把 used 从 false 翻成 true 的那次调用才算抢到。
// 并发派发时靠这一步保证同一账号不会同时发给两张卡密。
func reserveAccount(id uint) bool {
	res := database.DB.Model(&model.Account{}).
		Where("id = ? AND used = ?", id, false).
		Updates(map[string]interface{}{"used": true, "used_at": time.Now()})
	return res.Error == nil && res.RowsAffected == 1
}

// 释放预留：账号最终没有派发出去时归还号池。
func releaseAccount(id uint) {
	database.DB.Model(&model.Account{}).Where("id = ?", id).
		Updates(map[string]interface{}{"used": false, "used_at": nil})
}

func releaseAccounts(accounts []*model.Account) {
	for _, acc := range accounts {
		if acc != nil {
			releaseAccount(acc.ID)
		}
	}
}

// reservationGuard 跟踪"已预留但归属尚未确定"的账号。
//
// popAccount / popMultipleAccounts 会先把账号原子预留（used = true）再返回，
// 这样并发请求不会拿到同一个号。但预留之后到写入卡密绑定之前，中间任何提前返回
// （客户端断连、写库失败、panic）都会让账号停在 used = true 且没有任何绑定记录：
// 它既不在号池里，也不属于任何卡密，从后台看就是账号凭空少了。
//
// 用法：拿到预留账号后立即建立守卫并 defer Release，
// 每个账号一旦写入 CardAccount 绑定就调用 Commit。
// 函数无论从哪条路径退出，没 Commit 的都会被归还号池。
//
// Commit 的时机是"绑定成功"而不是"发送成功"：绑定一旦落库，账号归属就已确定，
// 用户重试时会通过卡密绑定关系重新取到它；此时再归还号池会让同一个号被派发两次。
type reservationGuard struct {
	pending map[uint]struct{}
}

func newReservationGuard(accounts []*model.Account) *reservationGuard {
	g := &reservationGuard{pending: make(map[uint]struct{}, len(accounts))}
	for _, acc := range accounts {
		if acc != nil {
			g.pending[acc.ID] = struct{}{}
		}
	}
	return g
}

func newReservationGuardOne(account *model.Account) *reservationGuard {
	return newReservationGuard([]*model.Account{account})
}

// Commit 标记账号归属已确定，不再需要归还。
func (g *reservationGuard) Commit(id uint) {
	delete(g.pending, id)
}

// Release 归还所有归属未确定的预留。必须用 defer 调用。
func (g *reservationGuard) Release() {
	for id := range g.pending {
		releaseAccount(id)
	}
	g.pending = nil
}

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
		// 账号已在 popMultipleAccounts 内部原子预留，任何提前返回都要归还未绑定的号。
		guard := newReservationGuard(accounts)
		defer guard.Release()

		result := database.DB.Model(&model.Card{}).Where("id = ? AND used_at IS NULL", card.ID).Update("used_at", now)
		if result.RowsAffected == 0 {
			// 卡密被并发抢先激活，预留全部由守卫归还。
			c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "卡密已被使用"})
			return
		}

		accountResps := make([]gin.H, 0, len(accounts))
		accountIDStrs := make([]string, 0, len(accounts))
		for _, acc := range accounts {
			if err := database.DB.Create(&model.CardAccount{CardID: card.ID, AccountID: acc.ID}).Error; err != nil {
				// 绑定失败则不交付这个号，交给守卫归还
				continue
			}
			guard.Commit(acc.ID)
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
		if len(accountResps) == 0 {
			// 一个都没绑定成功，卡密退回未使用
			database.DB.Model(&model.Card{}).Where("id = ?", card.ID).Update("used_at", nil)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "账号绑定失败，请重试"})
			return
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
	// 账号已在 popAccount 内部原子预留，任何提前返回都要归还。
	guard := newReservationGuardOne(account)
	defer guard.Release()

	result := database.DB.Model(&model.Card{}).Where("id = ? AND used_at IS NULL", card.ID).Updates(updates)
	if result.RowsAffected == 0 {
		// 卡密被并发抢先激活，预留由守卫归还。
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "卡密已被使用"})
		return
	}
	if err := database.DB.Create(&model.CardAccount{CardID: card.ID, AccountID: account.ID}).Error; err != nil {
		// 绑定失败：卡密退回未使用，账号由守卫归还，避免号被白占
		database.DB.Model(&model.Card{}).Where("id = ?", card.ID).Update("used_at", nil)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "账号绑定失败，请重试"})
		return
	}
	guard.Commit(account.ID)

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
			if !isDispatchable(&candidates[i], subscription) {
				continue
			}
			// 预留失败说明已被并发请求抢走，换下一个候选。
			if !reserveAccount(candidates[i].ID) {
				continue
			}
			account := candidates[i]
			return &account, nil
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
		if !reserveAccount(account.ID) {
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

	goSafe("dispatch-check-feed", func() {
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
				defer func() {
					if r := recover(); r != nil {
						log.Printf("派发健康检查 panic 已恢复 [account %d]: %v", acc.ID, r)
					}
				}()
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
				// 先原子预留再宣告胜出，避免同一账号被并发派发两次。
				// 预留失败说明已被抢走，让其他候选继续竞争。
				if !reserveAccount(acc.ID) {
					return
				}
				if found.CompareAndSwap(false, true) {
					winner <- &checkResult{account: acc}
					return
				}
				// 极少数情况下已有其他账号胜出，退回本次预留避免账号被白占。
				releaseAccount(acc.ID)
			}(unchecked[i])
		}
		wg.Wait()
	})

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
		"refreshToken": a.RefreshToken,
		"clientId":     a.ClientId,
		"clientSecret": a.ClientSecret,
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
			if !reserveAccount(candidates[i].ID) {
				continue
			}
			account := candidates[i]
			accounts = append(accounts, &account)
			if len(accounts) == n {
				return accounts, nil
			}
		}
		// 凑不齐 n 个，释放已预留的账号避免被白占。
		releaseAccounts(accounts)
		return nil, gorm.ErrRecordNotFound
	}

	accounts := make([]*model.Account, 0, n)
	freshCutoff := time.Now().Add(-20 * time.Minute)

	// 先挑出"近期检查过"的候选，再并发预检。
	// 串行预检时每个账号都要单独排一次全局上游队列，多号卡的等待时间会线性累加。
	fresh := make([]model.Account, 0, len(candidates))
	for i := range candidates {
		candidate := candidates[i]
		if candidate.LastCheckedAt == nil || !candidate.LastCheckedAt.After(freshCutoff) {
			continue
		}
		if !isDispatchable(&candidate, subscription) {
			continue
		}
		fresh = append(fresh, candidate)
	}

	if len(fresh) > 0 {
		type freshResult struct {
			account model.Account
			ok      bool
		}
		freshChan := make(chan freshResult, len(fresh))
		var freshWg sync.WaitGroup
		for i := range fresh {
			freshWg.Add(1)
			go func(acc model.Account) {
				defer freshWg.Done()
				// 预留成功后若发生 panic，必须把号还回去，否则它会停在
				// used = true 且无人认领。只在确实预留成功时才释放，
				// 避免误放别的请求刚抢到的号。
				reserved := false
				defer func() {
					if r := recover(); r != nil {
						log.Printf("多号卡预检 panic 已恢复 [account %d]: %v", acc.ID, r)
						if reserved {
							releaseAccount(acc.ID)
						}
						freshChan <- freshResult{account: acc}
					}
				}()

				if !verifyDispatchable(acc.AccessToken) {
					freshChan <- freshResult{account: acc}
					return
				}
				if !reserveAccount(acc.ID) {
					freshChan <- freshResult{account: acc}
					return
				}
				reserved = true
				freshChan <- freshResult{account: acc, ok: true}
			}(fresh[i])
		}
		freshWg.Wait()
		close(freshChan)

		// 凑够 n 个之后仍要排空，把多预留的账号还回池子。
		for r := range freshChan {
			if !r.ok {
				continue
			}
			if len(accounts) >= n {
				releaseAccount(r.account.ID)
				continue
			}
			acc := r.account
			accounts = append(accounts, &acc)
		}
	}

	// 如果已有足够的账号，直接返回
	if len(accounts) >= n {
		return accounts[:n], nil
	}

	// 批量并发健康检查未检查的账号
	unchecked := make([]model.Account, 0)
	for i := range candidates {
		if len(accounts) >= n {
			break
		}
		candidate := candidates[i]
		if candidate.LastCheckedAt != nil && candidate.LastCheckedAt.After(freshCutoff) {
			continue
		}
		unchecked = append(unchecked, candidate)
	}

	if len(unchecked) == 0 {
		if len(accounts) >= n {
			return accounts[:n], nil
		}
		releaseAccounts(accounts)
		return nil, gorm.ErrRecordNotFound
	}

	// 并发检查账号健康状态
	type checkedAccount struct {
		account model.Account
		valid   bool
	}
	
	resultChan := make(chan checkedAccount, len(unchecked))
	localLimit := currentUpstreamCheckConcurrency()
	if localLimit <= 0 {
		localLimit = 6
	}
	sem := make(chan struct{}, localLimit)
	var wg sync.WaitGroup

	for _, candidate := range unchecked {
		wg.Add(1)
		sem <- struct{}{}
		go func(acc model.Account) {
			defer func() { <-sem; wg.Done() }()
			// 与上面同理：预留后 panic 必须归还，否则号会凭空消失。
			reserved := false
			defer func() {
				if r := recover(); r != nil {
					log.Printf("多号卡健康检查 panic 已恢复 [account %d]: %v", acc.ID, r)
					if reserved {
						releaseAccount(acc.ID)
					}
					resultChan <- checkedAccount{account: acc, valid: false}
				}
			}()

			result := checkAccountHealth(acc)
			updates := buildHealthUpdates(result, time.Now())
			if err := persistHealthUpdates(acc.ID, updates); err != nil {
				resultChan <- checkedAccount{account: acc, valid: false}
				return
			}
			if result.errMsg != "" {
				resultChan <- checkedAccount{account: acc, valid: false}
				return
			}

			acc.AccessToken = result.newToken
			acc.RefreshToken = result.newRefresh
			acc.Email = result.email
			acc.Subscription = result.subscription
			acc.CreditUsed = result.creditUsed
			acc.CreditLimit = result.creditLimit
			if result.provider != "" {
				acc.Provider = result.provider
			}
			acc.Status = result.status
			
			if !isDispatchable(&acc, subscription) {
				resultChan <- checkedAccount{account: acc, valid: false}
				return
			}
			// 这里不再调用 verifyDispatchable：checkAccountHealth 的 Step 3 就是
			// ListAvailableModels，刚刚用同一个 accessToken 打过同一个端点，
			// 且只有 200 才会走到这里。重复探测不增加任何检测强度。
			if !reserveAccount(acc.ID) {
				resultChan <- checkedAccount{account: acc, valid: false}
				return
			}
			reserved = true

			resultChan <- checkedAccount{account: acc, valid: true}
		}(candidate)
	}

	goSafe("dispatch-multi-collect", func() {
		wg.Wait()
		close(resultChan)
	})

	// 收集有效账号。凑够 n 个后仍需排空 channel，
	// 把多余的已预留账号释放回池，否则它们会被白占。
	for checked := range resultChan {
		if !checked.valid {
			continue
		}
		if len(accounts) >= n {
			releaseAccount(checked.account.ID)
			continue
		}
		acc := checked.account
		accounts = append(accounts, &acc)
	}

	if len(accounts) < n {
		releaseAccounts(accounts)
		return nil, gorm.ErrRecordNotFound
	}
	return accounts[:n], nil
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
	// 已使用的卡密补号：卡密本身保持已用状态，不做回滚，
	// 未绑定的预留由 streamFillTokenAccounts 内部的守卫归还。
	ok, _, _ := streamFillTokenAccounts(c, item, item.total-sent, index)
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

	ok, sent, bound := streamFillTokenAccounts(c, item, item.total, index)
	if !ok && sent == 0 {
		// 一个都没送达，把卡密退回未使用。
		// 此时必须连同本次已写入的绑定一起撤销：否则卡密显示未使用，
		// 却挂着绑定记录且账号停在 used = true，下次提取会重新分配一批号，
		// 这一批就永远留在库里不属于任何人。
		rollbackCardBindings(item.card.ID, bound)
		database.DB.Model(&model.Card{}).Where("id = ?", item.card.ID).Update("used_at", nil)
	}
	return ok
}

// rollbackCardBindings 撤销本次为卡密建立的绑定，并把账号归还号池。
// 只用于"整笔提取都没成功、卡密要退回未使用"的场景。
func rollbackCardBindings(cardID uint, accountIDs []uint) {
	if len(accountIDs) == 0 {
		return
	}
	database.DB.Where("card_id = ? AND account_id IN ?", cardID, accountIDs).
		Delete(&model.CardAccount{})
	for _, id := range accountIDs {
		releaseAccount(id)
	}
}

// streamFillTokenAccounts 取号、绑定并流式下发。
// 返回：是否全部下发成功、实际下发数量、本次已绑定到该卡密的账号 ID。
func streamFillTokenAccounts(c *gin.Context, item tokenStreamCard, needed int, index *int) (bool, int, []uint) {
	if needed <= 0 {
		return true, 0, nil
	}
	clientIP := c.ClientIP()
	sent := 0
	accountIDStrs := make([]string, 0, needed)
	bound := make([]uint, 0, needed)

	// 批量获取所需数量的账号，避免重复调用 popAccount
	accounts, err := popMultipleAccounts(needed, item.card.Subscription)
	if err != nil || len(accounts) == 0 {
		streamTokenEvent(c, "fail", gin.H{
			"status":  http.StatusServiceUnavailable,
			"message": "账号池账号不足: " + item.code,
			"reason":  "account_pool_shortage",
			"partial": sent,
		})
		return false, sent, bound
	}

	// 账号已在 popMultipleAccounts 内部原子预留。
	// 客户端中途断连会让下面的循环提前返回，守卫负责把还没绑定的号还回池子，
	// 否则它们会停在 used = true 且无绑定，等于从号池里消失。
	guard := newReservationGuard(accounts)
	defer guard.Release()

	for _, account := range accounts {
		if err := database.DB.Create(&model.CardAccount{CardID: item.card.ID, AccountID: account.ID}).Error; err != nil {
			// 绑定失败，交给守卫归还
			continue
		}
		// 绑定已落库，归属确定，即使后面下发失败也不再归还
		guard.Commit(account.ID)
		bound = append(bound, account.ID)

		accountIDStrs = append(accountIDStrs, strconv.Itoa(int(account.ID)))
		database.DB.Create(&model.CardLog{CardID: item.card.ID, Code: item.card.Code, Action: "activate", AccountID: account.ID, Email: account.Email, ClientIP: clientIP})
		if !streamAccountToken(c, *index, item.code, account) {
			if len(accountIDStrs) > 0 {
				AddOpLogWithCtx(c, "activate", "流式提取中断 "+item.code+"，已绑定 "+strconv.Itoa(len(accountIDStrs))+" 个账号 ID:["+strings.Join(accountIDStrs, ",")+"], IP: "+clientIP, "client")
			}
			return false, sent, bound
		}
		*index = *index + 1
		sent++
	}

	if needed > 1 {
		AddOpLogWithCtx(c, "activate", "多号卡流式补号 "+item.code+"，绑定 "+strconv.Itoa(sent)+" 个账号 ID:["+strings.Join(accountIDStrs, ",")+"], IP: "+clientIP, "client")
	} else if len(accountIDStrs) > 0 {
		AddOpLogWithCtx(c, "activate", "凭证接口流式激活卡密 "+item.code+"，绑定账号 ID:"+accountIDStrs[0]+", IP: "+clientIP, "client")
	}
	return true, sent, bound
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
		// 账号已在 popMultipleAccounts 内部原子预留，任何提前返回都要归还未绑定的号。
		guard := newReservationGuard(accounts)
		defer guard.Release()

		result := database.DB.Model(&model.Card{}).Where("id = ? AND used_at IS NULL", card.ID).Update("used_at", now)
		if result.RowsAffected == 0 {
			// 卡密被并发抢先激活，预留全部由守卫归还。
			return nil, gin.H{"code": 1, "message": "卡密已被使用: " + code}, http.StatusBadRequest
		}

		idStrs := make([]string, 0, len(accounts))
		mods := make([]model.Account, 0, len(accounts))
		for _, acc := range accounts {
			if err := database.DB.Create(&model.CardAccount{CardID: card.ID, AccountID: acc.ID}).Error; err != nil {
				continue
			}
			guard.Commit(acc.ID)
			idStrs = append(idStrs, strconv.Itoa(int(acc.ID)))
			mods = append(mods, *acc)
			database.DB.Create(&model.CardLog{CardID: card.ID, Code: card.Code, Action: "activate", AccountID: acc.ID, Email: acc.Email, ClientIP: clientIP})
		}
		if len(mods) == 0 {
			database.DB.Model(&model.Card{}).Where("id = ?", card.ID).Update("used_at", nil)
			return nil, gin.H{"code": 1, "message": "账号绑定失败，请重试: " + code}, http.StatusInternalServerError
		}
		AddOpLogWithCtx(c, "activate", "多号卡激活 "+code+"，绑定 "+strconv.Itoa(len(accounts))+" 个账号 ID:["+strings.Join(idStrs, ",")+"], IP: "+clientIP, "client")
		return buildMultiTokenArray(mods), nil, 0
	}

	account, err := popAccount(0, card.Subscription)
	if err != nil {
		return nil, gin.H{"code": 2, "message": "账号池已空: " + code}, http.StatusServiceUnavailable
	}
	// 账号已在 popAccount 内部原子预留，任何提前返回都要归还。
	guard := newReservationGuardOne(account)
	defer guard.Release()

	result := database.DB.Model(&model.Card{}).Where("id = ? AND used_at IS NULL", card.ID).Update("used_at", now)
	if result.RowsAffected == 0 {
		// 卡密被并发抢先激活，预留由守卫归还。
		return nil, gin.H{"code": 1, "message": "卡密已被使用: " + code}, http.StatusBadRequest
	}
	if err := database.DB.Create(&model.CardAccount{CardID: card.ID, AccountID: account.ID}).Error; err != nil {
		database.DB.Model(&model.Card{}).Where("id = ?", card.ID).Update("used_at", nil)
		return nil, gin.H{"code": 1, "message": "账号绑定失败，请重试: " + code}, http.StatusInternalServerError
	}
	guard.Commit(account.ID)
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
