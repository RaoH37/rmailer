package rmailer

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
)

type Sender struct {
	UserName string
	Password string
	Host     string
}

func NewSender(u string, p string, s string) *Sender {
	return &Sender{
		UserName: u,
		Password: p,
		Host:     s,
	}
}

func (s *Sender) IsAuthenticated() bool {
	return len(s.Password) > 0
}

func (s *Sender) Send(m *Message) error {
	if s.IsAuthenticated() {
		return s.AuthenticatedSend(m)
	} else {
		return s.AnonymousSend(m)
	}
}

func (s *Sender) AnonymousSend(m *Message) error {
	log.Info(fmt.Sprintf("SMTP connection to %s with username %s", s.Host, s.UserName))

	c, err := smtp.Dial(s.Host)
	if err != nil {
		return err
	}
	defer c.Close()

	if err = c.Mail(s.UserName); err != nil {
		return err
	}
	defer c.Close()

	recipients(c, m)

	// Data
	w, err := c.Data()
	if err != nil {
		return err
	}
	defer c.Close()

	_, err = w.Write(m.ToBytes())
	if err != nil {
		return err
	}
	defer c.Close()

	err = w.Close()
	if err != nil {
		return err
	}
	defer c.Close()

	return c.Quit()
}

func (s *Sender) AuthenticatedSend(m *Message) error {
	log.Info(fmt.Sprintf("SMTP AUTH connection to %s", s.Host))

	host, _, _ := net.SplitHostPort(s.Host)

	auth := smtp.PlainAuth("", s.UserName, s.Password, host)

	tlsconfig := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         host,
	}

	conn, err := tls.Dial("tcp", s.Host, tlsconfig)
	if err != nil {
		return err
	}
	defer conn.Close()

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer c.Close()

	// Auth
	if err = c.Auth(auth); err != nil {
		return err
	}
	defer c.Close()

	if err = c.Mail(s.UserName); err != nil {
		return err
	}
	defer c.Close()

	recipients(c, m)

	// Data
	w, err := c.Data()
	if err != nil {
		return err
	}
	defer c.Close()

	_, err = w.Write(m.ToBytes())
	if err != nil {
		return err
	}
	defer c.Close()

	err = w.Close()
	if err != nil {
		return err
	}
	defer c.Close()

	return c.Quit()
}

func recipients(c *smtp.Client, m *Message) {
	for _, r := range m.To {
		if err := c.Rcpt(r.Address); err != nil {
			log.Println(err)
		}
	}

	for _, r := range m.CC {
		if err := c.Rcpt(r.Address); err != nil {
			log.Println(err)
		}
	}

	for _, r := range m.BCC {
		if err := c.Rcpt(r.Address); err != nil {
			log.Println(err)
		}
	}
}

type Message struct {
	From        mail.Address
	To          []mail.Address
	CC          []mail.Address
	BCC         []mail.Address
	Subject     string
	Body        string
	Attachments map[string][]byte
}

func (m *Message) SetFromFromString(s string) {
	m.From = mail.Address{Address: s}
}

func (m *Message) SetToFromStrings(ss []string) {
	m.To = make([]mail.Address, len(ss))

	for i, r := range ss {
		m.To[i] = mail.Address{Address: r}
	}
}

func (m *Message) SetCcFromStrings(ss []string) {
	m.CC = make([]mail.Address, len(ss))

	for i, r := range ss {
		m.CC[i] = mail.Address{Address: r}
	}
}

func (m *Message) SetBccFromStrings(ss []string) {
	m.BCC = make([]mail.Address, len(ss))

	for i, r := range ss {
		m.BCC[i] = mail.Address{Address: r}
	}
}

func NewMessage(subject, content string) *Message {
	return &Message{Subject: subject, Body: content, Attachments: make(map[string][]byte)}
}

func (m *Message) AttachFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	b, err := io.ReadAll(file)
	if err != nil {
		return err
	}
	defer file.Close()

	_, fileName := filepath.Split(path)
	m.Attachments[fileName] = b
	return nil
}

func (m *Message) ToBytes() []byte {
	buf := bytes.NewBuffer(nil)
	withAttachments := len(m.Attachments) > 0

	buf.WriteString(fmt.Sprintf("From: %s\r\n", m.From.String()))

	buf.WriteString(fmt.Sprintf("To: %s\r\n", get_recipients_str(m.To)))

	if len(m.CC) > 0 {
		buf.WriteString(fmt.Sprintf("Cc: %s\r\n", get_recipients_str(m.CC)))
	}

	var coder = base64.StdEncoding
	var subject = "=?UTF-8?B?" + coder.EncodeToString([]byte(m.Subject)) + "?="
	buf.WriteString("Subject: " + subject + "\r\n")

	buf.WriteString("MIME-Version: 1.0\n")
	writer := multipart.NewWriter(buf)
	boundary := writer.Boundary()
	if withAttachments {
		buf.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=%s\n", boundary))
		buf.WriteString(fmt.Sprintf("--%s\n", boundary))
	} else {
		buf.WriteString("Content-Type: text/plain; charset=utf-8\n")
	}

	buf.WriteString(fmt.Sprintf("Content-Type: %s; charset=utf-8\r\n\r\n", "text/plain"))
	buf.WriteString(m.Body)
	buf.WriteString("\r\n")

	if withAttachments {
		for k, v := range m.Attachments {
			buf.WriteString(fmt.Sprintf("\n\n--%s\n", boundary))

			contentType := getContentType(k, v)

			buf.WriteString(fmt.Sprintf("Content-Type: %s\n", contentType))
			buf.WriteString("Content-Transfer-Encoding: base64\n")

			buf.WriteString("Content-Disposition: attachment; filename=\"=?UTF-8?B?")
			buf.WriteString(coder.EncodeToString([]byte(k)))
			buf.WriteString("?=\"\r\n\r\n")

			b := make([]byte, base64.StdEncoding.EncodedLen(len(v)))
			base64.StdEncoding.Encode(b, v)

			// write base64 content in lines of up to 76 chars
			for i, l := 0, len(b); i < l; i++ {
				buf.WriteByte(b[i])
				if (i+1)%76 == 0 {
					buf.WriteString("\r\n")
				}
			}
			buf.WriteString(fmt.Sprintf("\n--%s", boundary))

		}

		buf.WriteString("--")
	}

	return buf.Bytes()
}

func getContentType(name string, content []byte) string {
	contentType := http.DetectContentType(content)
	if strings.HasPrefix(contentType, "text/plain") {
		ext := filepath.Ext(name)
		contentType = mime.TypeByExtension(ext)
	}

	return contentType
}

func get_recipients_str(recipients []mail.Address) string {
	recipients_str := []string{}

	for _, r := range recipients {
		recipients_str = append(recipients_str, r.String())
	}

	return strings.Join(recipients_str, ",")
}
