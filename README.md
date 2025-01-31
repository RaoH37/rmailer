# rmailer
library in go to send mails with attachments via smtp protocol

```go
package main

import (
  rmailer "github.com/RaoH37/rmailer"
  "net/mail"
)

func main() {
  sender := rmailer.NewSender("toto@toto.fr", "secret", "mta.toto.fr:465")
  
  htmlContent := "<p>It's <u>a good day</u> <i>to do nothing !</i></p>"
  txtContent := "It's a good day to do nothing !"

  m := rmailer.NewMessage("Hello", txtContent, htmlContent)

  m.From = mail.Address{Address: "toto@toto.fr"}
  m.To = []mail.Address{
    {Address: "tata@tata.fr"},
  }

  m.AttachFile("/tmp/file_1.txt")
  m.AttachFile("/tmp/file_2.txt")

  sender.Send(m)
}
```
