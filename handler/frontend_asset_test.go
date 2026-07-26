package handler

import (
	"os"
	"strings"
	"testing"
)

func TestProductImagePreviewHiddenRule(t *testing.T) {
	css, err := os.ReadFile("../static/css/main.css")
	if err != nil {
		t.Fatal(err)
	}
	content := strings.ReplaceAll(string(css), "\r\n", "\n")
	if !strings.Contains(content, ".product-image-preview[hidden]") || !strings.Contains(content, "display: none !important;") {
		t.Fatal("product image preview must remain hidden until an image is selected")
	}
}

func TestProductImagesUseContainWithoutForcedSizing(t *testing.T) {
	mainCSS, err := os.ReadFile("../static/css/main.css")
	if err != nil {
		t.Fatal(err)
	}
	shopCSS, err := os.ReadFile("../static/css/shop.css")
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"main.css": string(mainCSS), "shop.css": string(shopCSS)} {
		if !strings.Contains(content, "object-fit: contain;") || strings.Contains(content, "object-fit: cover;") {
			t.Fatalf("%s must show product images completely without cropping", name)
		}
	}
	if !strings.Contains(string(mainCSS), "width: 240px") || !strings.Contains(string(mainCSS), "height: 160px") {
		t.Fatal("admin preview needs a larger display frame")
	}
	if !strings.Contains(string(shopCSS), "height: 180px") {
		t.Fatal("shop product image needs a larger display frame")
	}
}
