# rmailer
library in go to send mails with attachments via smtp protocol

```go
package main

import (
  rmailer "github.com/RaoH37/rmailer"
)

func main() {
  config := NewSmtpConfig()

  sender := rmailer.NewSender(config.UserName, config.Password, config.ServerName, config.TLS)

  m := rmailer.NewMessage(config.subject(), config.content())

  m.To = config.to()
  m.CC = config.cc()
  m.BCC = config.bss()

  m.AttachFile("/tmp/file_1.txt")
  m.AttachFile("/tmp/file_2.txt")

  sender.Send(m)
}
```
