# rmailer
library in go to send mails with attachments via smtp protocol

```go
package main

import (
  rmailer "github.com/RaoH37/rmailer"
)

func main() {
  sender := rmailer.NewSender("toto@toto.fr", "secret", "mta.toto.fr:465")

  m := rmailer.NewMessage("Hello", "It's a good day to do nothing !")

  m.From = mail.Address{Address: "toto@toto.fr"}
  m.To = []mail.Address{
		{Address: "tata@tata.fr"},
	}

  m.AttachFile("/tmp/file_1.txt")
  m.AttachFile("/tmp/file_2.txt")

  sender.Send(m)
}
```
