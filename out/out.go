package out

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//Execute - provides out capability
func Execute(sourceRoot, version string, input []byte) (string, error) {
	var buildTokens = map[string]string{
		"${BUILD_ID}":            os.Getenv("BUILD_ID"),
		"${BUILD_NAME}":          os.Getenv("BUILD_NAME"),
		"${BUILD_JOB_NAME}":      os.Getenv("BUILD_JOB_NAME"),
		"${BUILD_PIPELINE_NAME}": os.Getenv("BUILD_PIPELINE_NAME"),
		"${ATC_EXTERNAL_URL}":    os.Getenv("ATC_EXTERNAL_URL"),
		"${BUILD_TEAM_NAME}":     os.Getenv("BUILD_TEAM_NAME"),
	}

	if sourceRoot == "" {
		return "", errors.New("expected path to build sources as first argument")
	}

	var indata Input

	err := json.Unmarshal(input, &indata)
	if err != nil {
		return "", err
	}

	if indata.Source.SMTP.Host == "" {
		return "", errors.New(`missing required field "source.smtp.host"`)
	}

	if indata.Source.SMTP.Port == "" {
		return "", errors.New(`missing required field "source.smtp.port"`)
	}

	if indata.Source.From == "" {
		return "", errors.New(`missing required field "source.from"`)
	}

	if len(indata.Source.To) == 0 && len(indata.Params.To) == 0 {
		return "", errors.New(`missing required field "source.to" or "params.to". Must specify at least one`)
	}

	if indata.Params.Subject == "" {
		return "", errors.New(`missing required field "params.subject"`)
	}

	if indata.Source.SMTP.Anonymous == false {
		if indata.Source.SMTP.Username == "" {
			return "", errors.New(`missing required field "source.smtp.username" if anonymous specify anonymous: true`)
		}

		if indata.Source.SMTP.Password == "" {
			return "", errors.New(`missing required field "source.smtp.password" if anonymous specify anonymous: true`)
		}
	}

	replaceTokens := func(sourceString string) string {
		for k, v := range buildTokens {
			sourceString = strings.Replace(sourceString, k, v, -1)
		}
		return sourceString
	}

	readSource := func(sourcePath string) (string, error) {
		if !filepath.IsAbs(sourcePath) {
			sourcePath = filepath.Join(sourceRoot, sourcePath)
		}
		var bytes []byte
		bytes, err = ioutil.ReadFile(sourcePath)
		return replaceTokens(string(bytes)), err
	}

	subject, err := readSource(indata.Params.Subject)
	if err != nil {
		return "", err
	}
	subject = strings.Trim(subject, "\n")

	var headers string
	if indata.Params.Headers != "" {
		headers, err = readSource(indata.Params.Headers)
		if err != nil {
			return "", err
		}
		headers = strings.Trim(headers, "\n")
	}

	var body string
	if indata.Params.Body != "" {
		body, err = readSource(indata.Params.Body)
		if err != nil {
			return "", err
		}
	}

	if indata.Params.To != "" {
		var toList string
		toList, err = readSource(indata.Params.To)
		if err != nil {
			return "", err
		}
		if len(toList) > 0 {
			toListArray := strings.Split(toList, ",")
			for _, toAddress := range toListArray {
				indata.Source.To = append(indata.Source.To, strings.TrimSpace(toAddress))
			}
		}
	}

	var outdata Output
	outdata.Version.Time = time.Now().UTC()
	outdata.Metadata = []MetadataItem{
		{Name: "smtp_host", Value: indata.Source.SMTP.Host},
		{Name: "subject", Value: subject},
		{Name: "version", Value: version},
	}
	outbytes, err := json.Marshal(outdata)
	if err != nil {
		return "", err
	}

	var messageData []byte
	messageData = append(messageData, []byte("To: "+strings.Join(indata.Source.To, ", ")+"\n")...)
	messageData = append(messageData, []byte("From: "+indata.Source.From+"\n")...)
	if headers != "" {
		messageData = append(messageData, []byte(headers+"\n")...)
	}
	messageData = append(messageData, []byte("Subject: "+subject+"\n")...)

	messageData = append(messageData, []byte("\n")...)
	messageData = append(messageData, []byte(body)...)

	if indata.Params.SendEmptyBody == false && len(body) == 0 {
		return string(outbytes), errors.New("Message not sent because the message body is empty and send_empty_body parameter was set to false. Github readme: https://github.com/pivotal-cf/email-resource")
	}

	if indata.Source.SMTP.Anonymous {
		err = smtp.SendMail(
			fmt.Sprintf("%s:%s", indata.Source.SMTP.Host, indata.Source.SMTP.Port),
			nil,
			indata.Source.From,
			indata.Source.To,
			messageData,
		)
	} else {
		err = smtp.SendMail(
			fmt.Sprintf("%s:%s", indata.Source.SMTP.Host, indata.Source.SMTP.Port),
			smtp.PlainAuth(
				"",
				indata.Source.SMTP.Username,
				indata.Source.SMTP.Password,
				indata.Source.SMTP.Host,
			),
			indata.Source.From,
			indata.Source.To,
			messageData,
		)
	}
	if err != nil {
		return "", err
	}

	return string(outbytes), nil
}