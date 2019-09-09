package mailyak

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"net/smtp"
	"regexp"
	"strings"
	"time"
)

// TODO: in the future, when aliasing is supported or we're making a breaking
// API change anyway, change the MailYak struct name to Email.

// MailYak represents an email.
type MailYak struct {
	html  BodyPart
	plain BodyPart

	toAddrs        []string
	ccAddrs        []string
	bccAddrs       []string
	subject        string
	fromAddr       string
	fromName       string
	replyTo        string
	headers        map[string]string // arbitrary headers
	attachments    []attachment
	auth           smtp.Auth
	trimRegex      *regexp.Regexp
	host           string
	writeBccHeader bool
	date           string
}

// add some expects for the various fields for testing
func (my *MailYak) GetToAddrs() []string          { return my.toAddrs }
func (my *MailYak) GetCCAddrs() []string          { return my.ccAddrs }
func (my *MailYak) GetBCCAddrs() []string         { return my.bccAddrs }
func (my *MailYak) GetSubject() string            { return my.subject }
func (my *MailYak) GetFromAddr() string           { return my.fromAddr }
func (my *MailYak) GetFromName() string           { return my.fromName }
func (my *MailYak) GetReplyTo() string            { return my.replyTo }
func (my *MailYak) GetHeaders() map[string]string { return my.headers }
func (my *MailYak) GetAttachments() []attachment  { return my.attachments }

// NewBlank returns an instance of MailYak
func NewBlank() *MailYak {
	return &MailYak{
		headers:        map[string]string{},
		trimRegex:      regexp.MustCompile("\r?\n"),
		writeBccHeader: false,
		date:           time.Now().Format(time.RFC1123Z),
	}
}

func (m *MailYak) Host(value string) {
	m.host = value
}

func (m *MailYak) Auth(value smtp.Auth) {
	m.auth = value
}

// New returns an instance of MailYak using host as the SMTP server, and
// authenticating with auth where required.
//
// host must include the port number (i.e. "smtp.itsallbroken.com:25")
//
// 		mail := mailyak.New("smtp.itsallbroken.com:25", smtp.PlainAuth(
// 			"",
// 			"username",
// 			"password",
// 			"stmp.itsallbroken.com",
//		))
//
func New(host string, auth smtp.Auth) *MailYak {
	return &MailYak{
		headers:        map[string]string{},
		host:           host,
		auth:           auth,
		trimRegex:      regexp.MustCompile("\r?\n"),
		writeBccHeader: false,
		date:           time.Now().Format(time.RFC1123Z),
	}
}

// Send attempts to send the built email via the configured SMTP server.
//
// Attachments are read when Send() is called, and any connection/authentication
// errors will be returned by Send().
func (m *MailYak) Send(localHostName string) (int, string, error) {

	buf, err := m.buildMime()
	if err != nil {
		return -1, "", err
	}

	// dial the host to get an smtp conn
	smtpClient, err := smtp.Dial(m.host)
	if err != nil {
		return -1, "", err
	}

	// make sure to quit client
	defer smtpClient.Close()

	// say hello to the smtp client
	if err = smtpClient.Hello(localHostName); err != nil {
		return -1, "", err
	}

	// if TLS is available use it
	if ok, _ := smtpClient.Extension("STARTTLS"); ok {
		config := &tls.Config{ServerName: localHostName}
		if err = smtpClient.StartTLS(config); err != nil {
			return -1, "", err
		}
	}

	// if we have auth
	if hasAuth, _ := smtpClient.Extension("AUTH"); hasAuth && m.auth != nil {
		smtpClient.Auth(m.auth)
	}

	// start the mailing
	if err = smtpClient.Mail(m.fromAddr); err != nil {
		return -1, "", err
	}

	// set the to addresses
	for _, addr := range m.toAddrs {
		if err = smtpClient.Rcpt(addr); err != nil {
			return -1, "", err
		}
	}

	// grab the underlying data writer
	w, err := smtpClient.Data()
	if err != nil {
		return -1, "", err
	}

	// write the email string
	_, err = w.Write(buf.Bytes())
	if err != nil {
		return -1, "", err
	}

	err = w.Close()
	if err != nil {
		return -1, "", err
	}

	// return the response from the smtpClient
	return smtpClient.Text.ReadResponse(0)
}

// MimeBuf returns the buffer containing all the RAW MIME data.
//
// MimeBuf is typically used with an API service such as Amazon SES that does
// not use an SMTP interface.
func (m *MailYak) MimeBuf() (*bytes.Buffer, error) {
	buf, err := m.buildMime()
	if err != nil {
		return nil, err
	}
	return buf, nil
}

// String returns a redacted description of the email state, typically for
// logging or debugging purposes.
//
// Authentication information is not included in the returned string.
func (m *MailYak) String() string {
	var (
		att    []string
		custom string
	)
	for _, a := range m.attachments {
		att = append(att, "{filename: "+a.filename+"}")
	}

	if len(m.headers) > 0 {
		var hdrs []string
		for k, v := range m.headers {
			hdrs = append(hdrs, fmt.Sprintf("%s: %q", k, v))
		}
		custom = strings.Join(hdrs, ", ") + ", "
	}
	return fmt.Sprintf(
		"&MailYak{date: %q, from: %q, fromName: %q, html: %v bytes, plain: %v bytes, toAddrs: %v, "+
			"bccAddrs: %v, subject: %q, %vhost: %q, attachments (%v): %v, auth set: %v}",
		m.date,
		m.fromAddr,
		m.fromName,
		len(m.HTML().String()),
		len(m.Plain().String()),
		m.toAddrs,
		m.bccAddrs,
		m.subject,
		custom,
		m.host,
		len(att),
		att,
		m.auth != nil,
	)
}

// HTML returns a BodyPart for the HTML email body.
func (m *MailYak) HTML() *BodyPart {
	return &m.html
}

// Plain returns a BodyPart for the plain-text email body.
func (m *MailYak) Plain() *BodyPart {
	return &m.plain
}
