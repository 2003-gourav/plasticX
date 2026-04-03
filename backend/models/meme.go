package models

import (
    "crypto/sha256"
    "encoding/hex"
    "fmt"
)

type Meme struct {
    ID         int    `json:"id"`
    CreatorID  string `json:"creator_id"`
    MarketID   int    `json:"market_id"`
    ImageURL   string `json:"image_url"`
    Caption    string `json:"caption"`
    ContentHash string `json:"content_hash"`
    CreatedAt  string `json:"created_at"`
}

func computeContentHash(imageURL, caption string) string {
    data := fmt.Sprintf("%s|%s", imageURL, caption)
    hash := sha256.Sum256([]byte(data))
    return hex.EncodeToString(hash[:])
}
