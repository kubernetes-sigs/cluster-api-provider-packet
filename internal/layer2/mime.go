package layer2

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/textproto"
	"strings"
)

var (
	cloudConfigType = textproto.MIMEHeader{
		"content-type": {"text/cloud-config"},
	}

	multipartHeader = strings.Join([]string{
		"MIME-Version: 1.0",
		"Content-Type: multipart/mixed; boundary=\"%s\"",
		"\n",
	}, "\n")
)

// GenerateInitDocument generates a multipart document with cloud-config data
func GenerateInitDocument(rawUserData, layer2UserData string) (string, error) {
	var buf bytes.Buffer
	mpWriter := multipart.NewWriter(&buf)
	buf.WriteString(fmt.Sprintf(multipartHeader, mpWriter.Boundary()))
	scriptWriter, err := mpWriter.CreatePart(cloudConfigType)
	if err != nil {
		return "", err
	}

	_, err = scriptWriter.Write([]byte(rawUserData))
	if err != nil {
		return "", err
	}

	includeWriter, err := mpWriter.CreatePart(cloudConfigType)
	if err != nil {
		return "", err
	}

	_, err = includeWriter.Write([]byte(layer2UserData))
	if err != nil {
		return "", err
	}

	if err := mpWriter.Close(); err != nil {
		return "", err
	}

	return buf.String(), nil
}
