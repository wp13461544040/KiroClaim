package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/wp13461544040/KiroClaim/subscription"
)

// SubToolGetStatus 公开端点：查询订阅工具是否对外开放
func SubToolGetStatus(c *gin.Context) {
	s := GetCurrentSettings()
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"enabled": s.SubToolEnabled}})
}

// SubToolGetPlans 公开端点：用调用方提供的账号凭据获取可用订阅计划
func SubToolGetPlans(c *gin.Context) {
	if !GetCurrentSettings().SubToolEnabled {
		c.JSON(http.StatusForbidden, gin.H{"code": 1, "message": "订阅工具未对外开放"})
		return
	}

	var req struct {
		Email        string `json:"email"`
		RefreshToken string `json:"refreshToken" binding:"required"`
		ClientID     string `json:"clientId"`
		ClientSecret string `json:"clientSecret"`
		Region       string `json:"region"`
		Provider     string `json:"provider"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "参数错误: " + err.Error()})
		return
	}

	acc := subscription.Account{
		Email:        req.Email,
		RefreshToken: req.RefreshToken,
		ClientID:     req.ClientID,
		ClientSecret: req.ClientSecret,
		Region:       req.Region,
		Provider:     req.Provider,
	}

	accessToken, err := subscription.RefreshAccessToken(acc)
	if err != nil {
		if subscription.IsSuspended(err) {
			c.JSON(http.StatusOK, gin.H{"code": 1, "message": "账号已被封禁"})
		} else {
			c.JSON(http.StatusOK, gin.H{"code": 1, "message": "刷新 token 失败: " + err.Error()})
		}
		return
	}

	result, err := subscription.ListPlansEx(acc, accessToken)
	if err != nil {
		if subscription.IsSuspended(err) {
			c.JSON(http.StatusOK, gin.H{"code": 1, "message": "账号已被封禁"})
		} else {
			c.JSON(http.StatusOK, gin.H{"code": 1, "message": "获取订阅计划失败: " + err.Error()})
		}
		return
	}

	// 优先用请求里的 email；没有则从 JWT 解码，再尝试 getUsageLimits
	email := req.Email
	if email == "" {
		email = subscription.ExtractEmailFromToken(accessToken)
	}
	currentSubscription := result.CurrentSubscription
	if gotEmail, gotSub, _, err2 := subscription.GetUserInfo(acc, accessToken); err2 == nil {
		if email == "" && gotEmail != "" {
			email = gotEmail
		}
		if currentSubscription == "" && gotSub != "" {
			currentSubscription = gotSub
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"plans":               result.Plans,
			"accessToken":         accessToken,
			"email":               email,
			"currentSubscription": currentSubscription,
		},
	})
}

// SubToolCreateLink 公开端点：用调用方提供的账号凭据创建订阅链接
func SubToolCreateLink(c *gin.Context) {
	if !GetCurrentSettings().SubToolEnabled {
		c.JSON(http.StatusForbidden, gin.H{"code": 1, "message": "订阅工具未对外开放"})
		return
	}

	var req struct {
		Email            string `json:"email"`
		RefreshToken     string `json:"refreshToken"  binding:"required"`
		ClientID         string `json:"clientId"`
		ClientSecret     string `json:"clientSecret"`
		Region           string `json:"region"`
		Provider         string `json:"provider"`
		AccessToken      string `json:"accessToken"   binding:"required"`
		SubscriptionType string `json:"subscriptionType"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "参数错误: " + err.Error()})
		return
	}

	acc := subscription.Account{
		Email:        req.Email,
		RefreshToken: req.RefreshToken,
		ClientID:     req.ClientID,
		ClientSecret: req.ClientSecret,
		Region:       req.Region,
		Provider:     req.Provider,
	}

	link, err := subscription.CreateSubscriptionLink(acc, req.AccessToken, req.SubscriptionType)
	if err != nil {
		if subscription.IsSuspended(err) {
			c.JSON(http.StatusOK, gin.H{"code": 1, "message": "账号已被封禁"})
		} else {
			c.JSON(http.StatusOK, gin.H{"code": 1, "message": "创建订阅链接失败: " + err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"link": link}})
}
