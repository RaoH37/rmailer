package rmailer

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
)

const (
	ContentTypeMultipartMixed          = "multipart/mixed"
	ContentTypeMultipartAlternative    = "multipart/alternative"
	ContentTypeTextHtml                = "text/html"
	ContentTypeTextPlain               = "text/plain"
	ContentTypeLine                    = "Content-Type: %s\n"
	ContentTypeLineBoundary            = "Content-Type: %s; boundary=%s\n\n--%s\n"
	ContentTransfertEncodingBase64Line = "Content-Transfer-Encoding: base64\n"
	MimeVersionLine                    = "MIME-Version: 1.0\n"
	BoundaryLine                       = "\n\n--%s\n"
	ContentDispositionAttachmentLine   = "Content-Disposition: attachment; filename=\"=?UTF-8?B?%s?=\"\r\n\r\n"
	BackLine                           = "\r\n"
)

type Sender struct {
	UserName string
	Password string
	Host     string
}

func NewSender(username string, password string, host string) *Sender {
	return &Sender{
		UserName: username,
		Password: password,
		Host:     host,
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
	log.Println(fmt.Sprintf("SMTP connection to %s with username %s", s.Host, s.UserName))

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
	log.Println(fmt.Sprintf("SMTP AUTH connection to %s", s.Host))

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
	BodyText    string
	BodyHtml    string
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

func NewMessage(subject, text string, html string) *Message {
	return &Message{
		Subject:     subject,
		BodyText:    text,
		BodyHtml:    html,
		Attachments: make(map[string][]byte),
	}
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
	var coder = base64.StdEncoding

	mb := &MessageBuilder{Message: m, Coder: coder}
	withAttachments := len(m.Attachments) > 0
	bothBody := len(m.BodyHtml) > 0 && len(m.BodyText) > 0

	buf := bytes.NewBuffer(nil)
	buf.WriteString(mb.FromLine())
	buf.WriteString(mb.ToLine())

	if len(m.CC) > 0 {
		buf.WriteString(mb.CcLine())
	}

	buf.WriteString(mb.SubjectLine())

	buf.WriteString(MimeVersionLine)

	writer := multipart.NewWriter(buf)
	boundaryMixed := writer.Boundary()
	boundaryAlternative := writer.Boundary()

	if withAttachments {
		buf.WriteString(fmt.Sprintf(ContentTypeLineBoundary, ContentTypeMultipartMixed, boundaryMixed, boundaryMixed))
	}

	if bothBody {
		buf.WriteString(fmt.Sprintf(ContentTypeLineBoundary, ContentTypeMultipartAlternative, boundaryAlternative, boundaryAlternative))
	}

	if len(m.BodyHtml) > 0 {
		buf.WriteString(mb.BodyHtmlLine())

		if len(m.BodyText) > 0 {
			buf.WriteString(fmt.Sprintf(BoundaryLine, boundaryAlternative))
		}
	}

	if len(m.BodyText) > 0 {
		buf.WriteString(mb.BodyTextLine())
	}

	if withAttachments {
		for k, v := range m.Attachments {
			buf.WriteString(fmt.Sprintf(BoundaryLine, boundaryMixed))

			buf.WriteString(fmt.Sprintf(ContentTypeLine, getContentType(k, v)))
			buf.WriteString(ContentTransfertEncodingBase64Line)
			buf.WriteString(fmt.Sprintf(ContentDispositionAttachmentLine, coder.EncodeToString([]byte(k))))

			b := make([]byte, base64.StdEncoding.EncodedLen(len(v)))
			base64.StdEncoding.Encode(b, v)

			// write base64 content in lines of up to 76 chars
			for i, l := 0, len(b); i < l; i++ {
				buf.WriteByte(b[i])
				if (i+1)%76 == 0 {
					buf.WriteString(BackLine)
				}
			}

			buf.WriteString(fmt.Sprintf(BoundaryLine, boundaryMixed))
		}

		buf.WriteString("--")
	}

	return buf.Bytes()
}

type MessageBuilder struct {
	Message *Message
	Coder   *base64.Encoding
}

func (mb *MessageBuilder) FromLine() string {
	return fmt.Sprintf("From: %s\r\n", mb.Message.From.String())
}

func (mb *MessageBuilder) ToLine() string {
	return fmt.Sprintf("To: %s\r\n", getRecipientsStr(mb.Message.To))
}

func (mb *MessageBuilder) CcLine() string {
	return fmt.Sprintf("Cc: %s\r\n", getRecipientsStr(mb.Message.CC))
}

func (mb *MessageBuilder) SubjectLine() string {
	var subjectUtf8 = mb.Coder.EncodeToString([]byte(mb.Message.Subject))
	return fmt.Sprintf("Subject: =?UTF-8?B?%s?=\r\n", subjectUtf8)
}

func (mb *MessageBuilder) BodyLine(content string, contentType string) string {
	return fmt.Sprintf("Content-Type: %s; charset=utf-8\r\n\r\n%s\r\n", contentType, content)
}

func (mb *MessageBuilder) BodyHtmlLine() string {
	return mb.BodyLine(mb.Message.BodyHtml, ContentTypeTextHtml)
}

func (mb *MessageBuilder) BodyTextLine() string {
	return mb.BodyLine(mb.Message.BodyText, ContentTypeTextPlain)
}

func getContentType(name string, content []byte) string {
	contentType := http.DetectContentType(content)
	if strings.HasPrefix(contentType, ContentTypeTextPlain) {
		ext := filepath.Ext(name)
		contentType = mime.TypeByExtension(ext)
	}

	return contentType
}

func getRecipientsStr(recipients []mail.Address) string {
	var recipientsStr []string

	for _, r := range recipients {
		recipientsStr = append(recipientsStr, r.String())
	}

	return strings.Join(recipientsStr, ",")
}
