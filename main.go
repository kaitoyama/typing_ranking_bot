package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
	traq "github.com/traPtitech/go-traq"
	traqwsbot "github.com/traPtitech/traq-ws-bot"
	payload "github.com/traPtitech/traq-ws-bot/payload"

	"database/sql"

	"github.com/go-sql-driver/mysql"
)

var (
	bot          *traqwsbot.Bot
	currentTop16 []ImageProc
)

type ImageProc struct {
	UserName      string  `json:"user_name"`
	Level         int     `json:"level"`
	MissTypeCount int     `json:"miss_type_count"`
	Speed         int     `json:"speed"`
	Accuracy      float32 `json:"accuracy"`
	Score         float32 `json:"score"`
}

func GetTop16(db *sql.DB, channelID string, forceOutput bool) {
	// top16のuserを取得してmdのテーブルとしてmessageに投稿する
	rows, err := db.Query(`
		WITH RankedScores AS (
			SELECT 
				user_name, 
				level, 
				miss_type_count, 
				speed, 
				accuracy, 
				score,
				ROW_NUMBER() OVER (PARTITION BY user_name ORDER BY score DESC) AS rank
			FROM 
				image_proc
		)
		SELECT 
			user_name, 
			level, 
			miss_type_count, 
			speed, 
			accuracy, 
			score AS best_score
		FROM 
			RankedScores
		WHERE 
			rank = 1
		ORDER BY 
			best_score DESC
		LIMIT 16;
	`)
	if err != nil {
		log.Println(err)
		return
	}
	defer rows.Close()

	top16 := make([]ImageProc, 0)
	for rows.Next() {
		var score ImageProc
		err := rows.Scan(&score.UserName, &score.Level, &score.MissTypeCount, &score.Speed, &score.Accuracy, &score.Score)
		if err != nil {
			log.Println(err)
			continue
		}
		top16 = append(top16, score)
	}

	// 今回のtop16がスコア含めて前回のtop16と異なる場合にのみ投稿するが、forceOutputがtrueの場合は無条件で投稿する
	if !forceOutput && len(top16) == len(currentTop16) {
		for i := 0; i < len(top16); i++ {
			if top16[i].UserName != currentTop16[i].UserName || top16[i].Level != currentTop16[i].Level || top16[i].MissTypeCount != currentTop16[i].MissTypeCount || top16[i].Speed != currentTop16[i].Speed || top16[i].Accuracy != currentTop16[i].Accuracy || top16[i].Score != currentTop16[i].Score {
				forceOutput = true
				break
			}
		}
	}

	// 人数が違うなら無条件で投稿する
	if !forceOutput && len(top16) != len(currentTop16) {
		forceOutput = true
	}

	if !forceOutput {
		return
	}

	currentTop16 = top16

	message := "## Top16が更新されました!\n| ユーザー名 | レベル | ミスタイプ数 | スピード | 正確性 | スコア |\n| --- | --- | --- | --- | --- | --- |\n"
	for _, score := range top16 {
		message += fmt.Sprintf("| %s | %d | %d | %d | %.3f | %.2f |\n", score.UserName, score.Level, score.MissTypeCount, score.Speed, score.Accuracy, score.Score)
	}

	_, _, err = bot.API().
		MessageApi.
		PostMessage(context.Background(), channelID).
		PostMessageRequest(traq.PostMessageRequest{
			Content: message,
		}).
		Execute()
	if err != nil {
		log.Println(err)
	}
}

func main() {
	if os.Getenv("nsapp_3c8306bdfe0c62cd462ed2") == "" {
		err := godotenv.Load()
		if err != nil {
			log.Println(err)
		}
	}

	jst, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		log.Println(err)
	}

	c := mysql.Config{
		DBName:               os.Getenv("NS_MARIADB_DATABASE"),
		User:                 os.Getenv("NS_MARIADB_USER"),
		Passwd:               os.Getenv("NS_MARIADB_PASSWORD"),
		Addr:                 os.Getenv("NS_MARIADB_HOSTNAME") + ":" + os.Getenv("NS_MARIADB_PORT"),
		Net:                  "tcp",
		ParseTime:            true,
		Collation:            "utf8mb4_unicode_ci",
		Loc:                  jst,
		AllowNativePasswords: true,
	}

	db, err := sql.Open("mysql", c.FormatDSN())
	if err != nil {
		log.Println(err)
	}

	defer db.Close()

	_, err = db.Exec("CREATE TABLE IF NOT EXISTS image_proc (id INT AUTO_INCREMENT PRIMARY KEY, user_name VARCHAR(255), level INT, miss_type_count INT, speed INT, accuracy FLOAT, score FLOAT, created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP)")

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
			if p.Message.Text == "!top16" {
				GetTop16(db, p.Message.ChannelID, true)
				return
			}
			// !fix id column value
			if p.Message.Text[:5] == "!fix " {
				// !fix id column value
				// spaceを使って分割してstringの配列にする
				s := p.Message.Text[5:]
				split := func(c rune) bool {
					return c == ' '
				}
				// 3つに分割する
				splitText := strings.FieldsFunc(s, split)
				// idをintに変換
				id, err := strconv.Atoi(splitText[0])
				if err != nil {
					log.Println(err)
					return
				}
				// column名
				cName := splitText[1]
				// value(型はinterface)
				value := splitText[2]
				// 修正
				fix(id, cName, value, db)
				GetTop16(db, p.Message.ChannelID, true)
				return
			}
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
			// message is like a JSON string
			// {"user_name":"sashabaranov","level":1,"miss_type_count":0,"speed":0,"accuracy":0}
			// We can parse it to a struct
			var score ImageProc
			err := json.Unmarshal([]byte(message), &score)
			if err != nil {
				log.Println(err)
			} else {
				score.Accuracy = score.Accuracy * 0.01
				if score.Level != 5 {
					log.Println("Received MESSAGE_CREATED event: " + p.Message.Text)
					_, _, err = bot.API().
						MessageApi.
						PostMessage(context.Background(), p.Message.ChannelID).
						PostMessageRequest(traq.PostMessageRequest{
							Content: "おっと、レベル5じゃないとダメだよ!",
						}).
						Execute()
					if err != nil {
						log.Println(err)
					}
				} else {
					log.Println("Received MESSAGE_CREATED event: " + p.Message.Text)
					_, err := db.Exec("INSERT INTO image_proc (user_name, level, miss_type_count, speed, accuracy, score) VALUES (?, ?, ?, ?, ?, ?)", score.UserName, score.Level, score.MissTypeCount, score.Speed, score.Accuracy, (float32(float32(score.Speed)*score.Accuracy-float32(score.MissTypeCount))*score.Accuracy)+float32(score.Speed))
					if err != nil {
						log.Println(err)
					}

					GetTop16(db, p.Message.ChannelID, false)
					_, _, err = bot.API().
						MessageApi.
						PostMessage(context.Background(), p.Message.ChannelID).
						PostMessageRequest(traq.PostMessageRequest{
							Content: fmt.Sprintf("以下の成績で受け付けました。何か誤りがある場合はkaitoyamaをメンションしてください\nユーザー名: %s\nレベル: %d\nミスタイプ数: %d\nスピード: %d\n正確性: %.3f\nスコア: %.2f", score.UserName, score.Level, score.MissTypeCount, score.Speed, score.Accuracy, (float32(float32(score.Speed)*score.Accuracy-float32(score.MissTypeCount))*score.Accuracy)+float32(score.Speed)),
						}).
						Execute()
					if err != nil {
						log.Println(err)
					}
				}
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

func fix(id int, cName string, value string, db *sql.DB) {
	fmt.Println(id, cName, value)
	if cName == "accuracy" {
		// valueをfloatに変換
		f, err := strconv.ParseFloat(value, 32)
		if err != nil {
			log.Println(err)
		}
		// idとカラムと修正後の値をうけて、その項目とスコアを修正する
		query := fmt.Sprintf("UPDATE image_proc SET %s = ? WHERE id = ?", cName)
		_, err = db.Exec(query, f, id)
		if err != nil {
			log.Println(err)
		}

	} else {
		// valueをintに変換
		i, err := strconv.Atoi(value)
		if err != nil {
			log.Println(err)
		}
		// idとカラムと修正後の値をうけて、その項目とスコアを修正する
		query := fmt.Sprintf("UPDATE image_proc SET %s = ? WHERE id = ?", cName)
		_, err = db.Exec(query, i, id)
		if err != nil {
			log.Println(err)
		}

	}

	// 修正後のスコアを計算して修正する
	rows, err := db.Query("SELECT * FROM image_proc WHERE id = ?", id)
	if err != nil {
		log.Println(err)
	}
	defer rows.Close()

	for rows.Next() {
		var score ImageProc
		err := rows.Scan(&score.UserName, &score.Level, &score.MissTypeCount, &score.Speed, &score.Accuracy, &score.Score)
		if err != nil {
			log.Println(err)
		}
		_, err = db.Exec("UPDATE image_proc SET score = ? WHERE id = ?", (float32(float32(score.Speed)*score.Accuracy-float32(score.MissTypeCount))*score.Accuracy)+float32(score.Speed), id)
		if err != nil {
			log.Println(err)
		}
	}
}
