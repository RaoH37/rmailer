package rmailer

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"path/filepath"
	"strings"
)

type Sender struct {
	UserName   string
	Password   string
	ServerName string
	TLS        bool
}

func NewSender(u string, p string, s string, t bool) *Sender {
	return &Sender{UserName: u, Password: p, ServerName: s, TLS: t}
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
	// fmt.Println(fmt.Sprintf("SMTP ANONYMOUS vers le serveur %s\n", s.ServerName))

	c, err := smtp.Dial(s.ServerName)
	if err != nil {
		log.Panic(err)
	}

	// To && From
	if err = c.Mail(s.UserName); err != nil {
		log.Panic(err)
	}

	for _, to := range m.To {
		if err = c.Rcpt(to.Address); err != nil {
			log.Panic(err)
		}
	}

	// Data
	w, err := c.Data()
	if err != nil {
		log.Panic(err)
	}

	_, err = w.Write(m.ToBytes())
	if err != nil {
		log.Panic(err)
	}

	err = w.Close()
	if err != nil {
		log.Panic(err)
	}

	return c.Quit()
}

func (s *Sender) AuthenticatedSend(m *Message) error {
	fmt.Println(fmt.Sprintf("SMTP AUTH vers le serveur %s\n", s.ServerName))
	host, _, _ := net.SplitHostPort(s.ServerName)

	auth := smtp.PlainAuth("", s.UserName, s.Password, host)

	tlsconfig := &tls.Config{
		InsecureSkipVerify: s.TLS,
		ServerName:         host,
	}

	conn, err := tls.Dial("tcp", s.ServerName, tlsconfig)
	if err != nil {
		log.Panic(err)
	}

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		log.Panic(err)
	}

	// Auth
	if err = c.Auth(auth); err != nil {
		log.Panic(err)
	}

	// To && From
	if err = c.Mail(s.UserName); err != nil {
		log.Panic(err)
	}

	for _, to := range m.To {
		if err = c.Rcpt(to.Address); err != nil {
			log.Panic(err)
		}
	}

	// Data
	w, err := c.Data()
	if err != nil {
		log.Panic(err)
	}

	_, err = w.Write(m.ToBytes())
	if err != nil {
		log.Panic(err)
	}

	err = w.Close()
	if err != nil {
		log.Panic(err)
	}

	return c.Quit()
}

type Message struct {
	To          []mail.Address
	CC          []mail.Address
	BCC         []mail.Address
	Subject     string
	Body        string
	Attachments map[string][]byte
}

func NewMessage(s, b string) *Message {
	return &Message{Subject: s, Body: b, Attachments: make(map[string][]byte)}
}

func (m *Message) AttachFile(src string) error {
	b, err := ioutil.ReadFile(src)
	if err != nil {
		return err
	}

	_, fileName := filepath.Split(src)
	m.Attachments[fileName] = b
	return nil
}

func (m *Message) ToBytes() []byte {
	buf := bytes.NewBuffer(nil)
	withAttachments := len(m.Attachments) > 0
	var coder = base64.StdEncoding
	var subject = "=?UTF-8?B?" + coder.EncodeToString([]byte(m.Subject)) + "?="
	buf.WriteString("Subject: " + subject + "\r\n")

	// buf.WriteString(fmt.Sprintf("Subject: %s\n", m.Subject))
	buf.WriteString(fmt.Sprintf("To: %s\r\n", get_recipients_str(m.To)))

	if len(m.CC) > 0 {
		buf.WriteString(fmt.Sprintf("Cc: %s\r\n", get_recipients_str(m.CC)))
	}

	if len(m.BCC) > 0 {
		buf.WriteString(fmt.Sprintf("Bcc: %s\r\n", get_recipients_str(m.BCC)))
	}

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
