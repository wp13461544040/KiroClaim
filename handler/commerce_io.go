package handler

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/huey1in/KiroClaim/database"
	"github.com/huey1in/KiroClaim/model"

	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"gorm.io/gorm"
)

var allowedProofTypes = map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp", "application/pdf": ".pdf"}

func storeCommerceProof(file *multipart.FileHeader, settings commerceSettings, orderNo string) (string, error) {
	src, err := file.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()
	buf := make([]byte, 512)
	n, _ := io.ReadFull(src, buf)
	contentType := httpDetect(buf[:n])
	ext, ok := allowedProofTypes[contentType]
	if !ok {
		return "", errors.New("仅支持 JPG、PNG、WebP 或 PDF")
	}
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	token, err := randomCommerceToken(12)
	if err != nil {
		return "", err
	}
	objectName := orderNo + "/" + strings.ToLower(token) + ext
	if settings.StorageType == "s3" {
		client, err := minio.New(settings.S3Endpoint, &minio.Options{Creds: credentials.NewStaticV4(settings.S3AccessKey, settings.S3SecretKey, ""), Secure: settings.S3UseSSL, Region: settings.S3Region})
		if err != nil {
			return "", err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		exists, err := client.BucketExists(ctx, settings.S3Bucket)
		if err != nil {
			return "", err
		}
		if !exists {
			if err := client.MakeBucket(ctx, settings.S3Bucket, minio.MakeBucketOptions{Region: settings.S3Region}); err != nil {
				return "", err
			}
		}
		_, err = client.PutObject(ctx, settings.S3Bucket, objectName, src, file.Size, minio.PutObjectOptions{ContentType: contentType})
		if err != nil {
			return "", err
		}
		return "s3:" + objectName, nil
	}
	base, err := filepath.Abs(settings.LocalPath)
	if err != nil {
		return "", err
	}
	target := filepath.Join(base, filepath.FromSlash(objectName))
	if !strings.HasPrefix(target, base+string(os.PathSeparator)) {
		return "", errors.New("invalid proof path")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return "", err
	}
	dst, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	defer dst.Close()
	if _, err = io.Copy(dst, src); err != nil {
		return "", err
	}
	return "local:" + objectName, nil
}

func httpDetect(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return http.DetectContentType(b)
}

func SubmitShopPaymentProof(c *gin.Context) {
	settings := loadCommerceSettings()
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, (settings.MaxProofMB*5+1)<<20)
	orderNo := strings.TrimSpace(c.PostForm("orderNo"))
	password := c.PostForm("password")
	var order model.CommerceOrder
	if database.DB.Where("order_no=?", orderNo).First(&order).Error != nil || !verifyOrderAccess(&order, password) {
		c.JSON(404, gin.H{"code": 1, "message": "订单号或查询密码错误"})
		return
	}
	if order.Status != model.OrderPendingPayment && order.Status != model.OrderPaymentReview {
		c.JSON(409, gin.H{"code": 1, "message": "当前订单不能提交凭证"})
		return
	}
	channel, ok := findCommerceChannel(order.ChannelID, false)
	if !ok || channel.ChannelType != "manual" {
		c.JSON(409, gin.H{"code": 1, "message": "该订单不是人工支付"})
		return
	}
	rules := parseJSON[commercePublicConfig](channel.PublicConfig)
	reference := strings.TrimSpace(c.PostForm("reference"))
	payer := strings.TrimSpace(c.PostForm("payerInfo"))
	note := strings.TrimSpace(c.PostForm("note"))
	if rules.RequireReference && reference == "" || rules.RequirePayer && payer == "" || rules.RequireNote && note == "" {
		c.JSON(400, gin.H{"code": 1, "message": "付款凭证字段不完整"})
		return
	}
	if err := c.Request.ParseMultipartForm(settings.MaxProofMB << 20); err != nil {
		c.JSON(400, gin.H{"code": 1, "message": "凭证文件过大"})
		return
	}
	files := c.Request.MultipartForm.File["proofs"]
	if rules.RequireProof && len(files) == 0 {
		c.JSON(400, gin.H{"code": 1, "message": "请上传付款截图"})
		return
	}
	if len(files) > 5 {
		c.JSON(400, gin.H{"code": 1, "message": "最多上传 5 个文件"})
		return
	}
	stored := make([]string, 0, len(files))
	for _, f := range files {
		if f.Size > settings.MaxProofMB<<20 {
			c.JSON(400, gin.H{"code": 1, "message": "单个文件过大"})
			return
		}
		path, err := storeCommerceProof(f, settings, order.OrderNo)
		if err != nil {
			c.JSON(400, gin.H{"code": 1, "message": err.Error()})
			return
		}
		stored = append(stored, path)
	}
	fileJSON, _ := json.Marshal(stored)
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		proof := model.CommercePaymentProof{OrderID: order.ID, Reference: reference, PayerInfo: payer, Note: note, FilesJSON: string(fileJSON)}
		if err := tx.Create(&proof).Error; err != nil {
			return err
		}
		return tx.Model(&order).Updates(map[string]interface{}{"status": model.OrderPaymentReview, "review_note": "付款凭证待审核"}).Error
	})
	if err != nil {
		c.JSON(500, gin.H{"code": 1, "message": err.Error()})
		return
	}
	c.JSON(200, gin.H{"code": 0, "message": "付款凭证已提交，等待管理员审核"})
}

func AdminCommerceProof(c *gin.Context) {
	proofID, err := strconv.ParseUint(c.Param("proofId"), 10, 64)
	index, indexErr := strconv.Atoi(c.Param("index"))
	if err != nil || indexErr != nil || index < 0 {
		c.JSON(400, gin.H{"code": 1})
		return
	}
	var proof model.CommercePaymentProof
	if database.DB.First(&proof, uint(proofID)).Error != nil {
		c.JSON(404, gin.H{"code": 1})
		return
	}
	var files []string
	if json.Unmarshal([]byte(proof.FilesJSON), &files) != nil || index >= len(files) {
		c.JSON(404, gin.H{"code": 1})
		return
	}
	settings := loadCommerceSettings()
	stored := files[index]
	if strings.HasPrefix(stored, "s3:") {
		name := strings.TrimPrefix(stored, "s3:")
		client, err := minio.New(settings.S3Endpoint, &minio.Options{Creds: credentials.NewStaticV4(settings.S3AccessKey, settings.S3SecretKey, ""), Secure: settings.S3UseSSL, Region: settings.S3Region})
		if err != nil {
			c.JSON(500, gin.H{"code": 1})
			return
		}
		object, err := client.GetObject(c.Request.Context(), settings.S3Bucket, name, minio.GetObjectOptions{})
		if err != nil {
			c.JSON(404, gin.H{"code": 1})
			return
		}
		defer object.Close()
		stat, err := object.Stat()
		if err != nil {
			c.JSON(404, gin.H{"code": 1})
			return
		}
		c.DataFromReader(200, stat.Size, stat.ContentType, object, map[string]string{"Content-Disposition": "inline"})
		return
	}
	base, _ := filepath.Abs(settings.LocalPath)
	target, _ := filepath.Abs(filepath.Join(base, filepath.FromSlash(strings.TrimPrefix(stored, "local:"))))
	if !strings.HasPrefix(target, base+string(os.PathSeparator)) {
		c.JSON(400, gin.H{"code": 1})
		return
	}
	c.File(target)
}

func sendCommerceCompletionEmail(order model.CommerceOrder) {
	s := loadCommerceSettings()
	if !s.EmailOnComplete || s.SMTPHost == "" || s.SMTPFrom == "" || !strings.Contains(order.Contact, "@") {
		return
	}
	subject := "KiroClaim 订单已完成 " + order.OrderNo
	body := "您的订单已完成。\r\n订单号: " + order.OrderNo + "\r\n请使用订单查询密码在商城查询页领取卡密。\r\n"
	if s.EmailIncludeCard {
		if codes, err := commerceOrderCardCodes(database.DB, order.ID); err == nil {
			for _, code := range codes {
				body += "卡密: " + code + "\r\n"
			}
		}
	}
	message := []byte("From: " + s.SMTPFrom + "\r\nTo: " + order.Contact + "\r\nSubject: " + subject + "\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + body)
	addr := net.JoinHostPort(s.SMTPHost, strconv.Itoa(s.SMTPPort))
	auth := smtp.PlainAuth("", s.SMTPUsername, s.SMTPPassword, s.SMTPHost)
	var err error
	if s.SMTPTLSMode == "tls" {
		var conn *tls.Conn
		conn, err = tls.Dial("tcp", addr, &tls.Config{ServerName: s.SMTPHost, MinVersion: tls.VersionTLS12})
		if err == nil {
			var client *smtp.Client
			client, err = smtp.NewClient(conn, s.SMTPHost)
			if err == nil {
				if s.SMTPUsername != "" {
					err = client.Auth(auth)
				}
				if err == nil {
					err = client.Mail(s.SMTPFrom)
				}
				if err == nil {
					err = client.Rcpt(order.Contact)
				}
				if err == nil {
					var writer io.WriteCloser
					writer, err = client.Data()
					if err == nil {
						_, err = writer.Write(message)
						_ = writer.Close()
					}
				}
				_ = client.Quit()
			}
		}
	} else if s.SMTPTLSMode == "starttls" {
		client, dialErr := smtp.Dial(addr)
		if dialErr != nil {
			err = dialErr
		} else {
			defer client.Close()
			err = client.StartTLS(&tls.Config{ServerName: s.SMTPHost, MinVersion: tls.VersionTLS12})
			if err == nil && s.SMTPUsername != "" {
				err = client.Auth(auth)
			}
			if err == nil {
				err = client.Mail(s.SMTPFrom)
			}
			if err == nil {
				err = client.Rcpt(order.Contact)
			}
			if err == nil {
				var writer io.WriteCloser
				writer, err = client.Data()
				if err == nil {
					_, err = writer.Write(message)
					_ = writer.Close()
				}
			}
			if err == nil {
				err = client.Quit()
			}
		}
	} else {
		err = smtp.SendMail(addr, auth, s.SMTPFrom, []string{order.Contact}, message)
	}
	if err != nil {
		writeOpLog("commerce_mail", "订单 "+order.OrderNo+" 邮件发送失败: "+err.Error(), "system", "", "")
	}
}
