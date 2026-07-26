package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPageRoutesUseRootForShopAndAdminForConsole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerPageRoutes(router)

	tests := []struct {
		path string
		want string
	}{
		{path: "/", want: "在线购买卡密"},
		{path: "/admin", want: "登录"},
	}
	for _, test := range tests {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), test.want) {
			t.Fatalf("GET %s = %d, body missing %q", test.path, recorder.Code, test.want)
		}
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/shop", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("GET /shop = %d, want 404", recorder.Code)
	}
}
