package main

import (
	"context"
	"encoding/base64"
	"io"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/traPtitech/go-traq"
	traqwsbot "github.com/traPtitech/traq-ws-bot"
)

func GetBot() *traqwsbot.Bot {
	return bot
}

func GetFileMetadata(fileID string) *traq.FileInfo {
	bot := GetBot()

	fileInfo, _, err := bot.API().
		FileApi.GetFileMeta(context.Background(), fileID).
		Execute()
	if err != nil {
		log.Println(err)
	}

	return fileInfo
}

func GetFileData(fileID string) *os.File {
	bot := GetBot()

	fileData, _, err := bot.API().
		FileApi.GetFile(context.Background(), fileID).
		Execute()
	if err != nil {
		log.Println(err)
	}

	return fileData
}

func ConvertFileToBase64IfFileIsImage(fileID string) *string {
	if !isImage(fileID) {
		log.Println("Not an image")

		return nil
	}

	fileData := GetFileData(fileID)
	if fileData == nil {
		log.Println("Failed to get file data")

		return nil
	}
	defer fileData.Close()

	base64Data, err := fileToBase64(fileData)
	if err != nil {
		log.Printf("Error reading file: %v\n", err)

		return nil
	}

	return base64Data
}

// isImage は、MIMEタイプが画像であるかを判定する関数
func isImage(fileID string) bool {
	fileInfo := GetFileMetadata(fileID)
	if fileInfo == nil {
		log.Println("Failed to get file metadata")

		return false
	}

	return strings.HasPrefix(fileInfo.Mime, "image/")
}

// fileToBase64 は、ファイルを読み込み、BASE64エンコードされた文字列に変換する関数
func fileToBase64(file *os.File) (*string, error) {
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	base64Data := base64.StdEncoding.EncodeToString(data)

	return &base64Data, nil
}

func ExtractFileUUIDs(text string) []string {
	const pattern = `https://q\.trap\.jp/files/([a-fA-F0-9-]{36})`

	re := regexp.MustCompile(pattern)
	matches := re.FindAllStringSubmatch(text, -1)

	var uuids []string
	for _, match := range matches {
		if len(match) > 1 {
			uuids = append(uuids, match[1])
		}
	}

	return uuids
}

func GetBase64ImagesFromMessage(message string) []string {
	fileUUIDs := ExtractFileUUIDs(message)
	var base64Images []string

	for _, uuid := range fileUUIDs {
		base64Data := ConvertFileToBase64IfFileIsImage(uuid)
		if base64Data != nil {
			base64Images = append(base64Images, *base64Data)
		}
	}

	return base64Images
}
