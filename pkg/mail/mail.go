// Package mail SMTP 邮件发送服务
// 配置来源：settings 中的 mail 配置段（可通过环境变量 MAIL_HOST 等覆盖，viper AutomaticEnv）
package mail

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

var conf Config

var disabled bool // 未配置 SMTP 时为 true，此时发送验证码直接报错但不阻塞服务启动

// Config SMTP 配置
type Config struct {
	Host     string // SMTP 服务器地址
	Port     int    // 端口（465 隐式 TLS，其他端口走 STARTTLS）
	Username string // 认证用户名
	Password string // 认证密码（不落库、不进仓库，Docker Compose 通过 .env 注入）
	From     string // 发件人地址
	TLS      bool   // 是否启用 TLS
}

// Init 从配置初始化邮件服务
func Init() error {
	conf = Config{
		Host:     viper.GetString("mail.host"),
		Port:     viper.GetInt("mail.port"),
		Username: viper.GetString("mail.username"),
		Password: viper.GetString("mail.password"),
		From:     viper.GetString("mail.from"),
		TLS:      viper.GetBool("mail.tls"),
	}
	if conf.Host == "" || conf.Port == 0 || conf.From == "" {
		// SMTP 未配置时不阻塞启动，只在真正发信时返回错误
		disabled = true
		return nil
	}
	return nil
}

// SendCodeMail 发送验证码邮件
func SendCodeMail(to, code string) error {
	subject := "Bluebell 知识社区 - 邮箱验证码"
	body := fmt.Sprintf(`
<div style="max-width:480px;margin:0 auto;font-family:-apple-system,'PingFang SC','Microsoft YaHei',sans-serif;">
  <h2 style="color:#1a1a1a;">Bluebell 知识社区</h2>
  <p style="color:#646464;">你的验证码是：</p>
  <p style="font-size:28px;font-weight:700;letter-spacing:6px;color:#0084ff;">%s</p>
  <p style="color:#999;font-size:13px;">验证码 5 分钟内有效，请勿泄露给他人。</p>
</div>`, code)
	return send(to, subject, body)
}

// send 发送邮件：465 端口走隐式 TLS，其余端口由 smtp.SendMail 自动协商 STARTTLS
func send(to, subject, htmlBody string) error {
	if disabled {
		return fmt.Errorf("邮件服务未配置（请设置 MAIL_HOST/MAIL_PORT/MAIL_FROM 等环境变量）")
	}
	addr := conf.Host + ":" + strconv.Itoa(conf.Port)
	from := conf.From

	header := make(map[string]string)
	header["From"] = from
	header["To"] = to
	header["Subject"] = subject
	header["MIME-Version"] = "1.0"
	header["Content-Type"] = "text/html; charset=UTF-8"
	header["Date"] = time.Now().Format(time.RFC1123Z)

	var sb strings.Builder
	for k, v := range header {
		sb.WriteString(k + ": " + v + "\r\n")
	}
	sb.WriteString("\r\n" + htmlBody)
	msg := []byte(sb.String())

	auth := smtp.PlainAuth("", conf.Username, conf.Password, conf.Host)

	// 465 端口使用隐式 TLS 连接
	if conf.TLS || conf.Port == 465 {
		return sendWithImplicitTLS(addr, auth, from, to, msg)
	}
	return smtp.SendMail(addr, auth, from, []string{to}, msg)
}

// sendWithImplicitTLS 通过隐式 TLS（SSL）连接发送邮件
func sendWithImplicitTLS(addr string, auth smtp.Auth, from, to string, msg []byte) error {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("dial smtp failed: %w", err)
	}
	host, _, _ := net.SplitHostPort(addr)
	tlsConn := tls.Client(conn, &tls.Config{ServerName: host})
	client, err := smtp.NewClient(tlsConn, host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("smtp client failed: %w", err)
	}
	defer client.Close()

	if err = client.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth failed: %w", err)
	}
	if err = client.Mail(from); err != nil {
		return err
	}
	if err = client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err = w.Write(msg); err != nil {
		return err
	}
	if err = w.Close(); err != nil {
		return err
	}
	return client.Quit()
}
