package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
	traq "github.com/traPtitech/go-traq"
	traqwsbot "github.com/traPtitech/traq-ws-bot"
	payload "github.com/traPtitech/traq-ws-bot/payload"
)

var (
	bot *traqwsbot.Bot
)

type ImageProc struct {
	UserName      string  `json:"user_name"`
	Level         int     `json:"level"`
	MissTypeCount int     `json:"miss_type_count"`
	Speed         int     `json:"speed"`
	Accuracy      float32 `json:"accuracy"`
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println(err)
	}

	ACCESS_TOKEN := os.Getenv("ACCESS_TOKEN")
	bot, err = traqwsbot.NewBot(&traqwsbot.Options{
		AccessToken: ACCESS_TOKEN,
	})
	if err != nil {
		panic(err)
	}

	bot.OnMessageCreated(func(p *payload.MessageCreated) {
		images := GetBase64ImagesFromMessage(p.Message.Text)
		if len(images) == 0 {
			_, _, err := bot.API().
				MessageApi.
				PostMessage(context.Background(), p.Message.ChannelID).
				PostMessageRequest(traq.PostMessageRequest{
					Content: "No image found",
				}).
				Execute()
			if err != nil {
				log.Println(err)
			}
		} else {
			message := image_proc(images[0])
			log.Println("Received MESSAGE_CREATED event: " + p.Message.Text)
			_, _, err := bot.API().
				MessageApi.
				PostMessage(context.Background(), p.Message.ChannelID).
				PostMessageRequest(traq.PostMessageRequest{
					Content: message,
				}).
				Execute()
			if err != nil {
				log.Println(err)
			}
		}
	})

	if err := bot.Start(); err != nil {
		panic(err)
	}
}

func image_proc(imagebasee64 string) string {
	TOKEN := os.Getenv("OPENAI_API_KEY")
	client := openai.NewClient(TOKEN)

	part := openai.ChatMessagePart{}
	part.Type = openai.ChatMessagePartTypeImageURL
	part.ImageURL = &openai.ChatMessageImageURL{
		URL:    "data:image/jpeg;base64," + imagebasee64,
		Detail: openai.ImageURLDetailLow,
	}

	schema, err := jsonschema.GenerateSchemaForType(ImageProc{})
	if err != nil {
		log.Fatalf("GenerateSchemaForType error: %v", err)
	}

	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT4o20240806,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: "",
					MultiContent: []openai.ChatMessagePart{
						part,
					},
				},
			},
			ResponseFormat: &openai.ChatCompletionResponseFormat{
				Type: openai.ChatCompletionResponseFormatTypeJSONSchema,
				JSONSchema: &openai.ChatCompletionResponseFormatJSONSchema{
					Strict: true,
					Name:   "ImageProc",
					Schema: schema,
				},
			},
		},
	)

	if err != nil {
		fmt.Printf("ChatCompletion error: %v\n", err)
		return ""
	}

	return resp.Choices[0].Message.Content
}
